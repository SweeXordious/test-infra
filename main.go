package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/celestiaorg/celestia-node/logs"
	"github.com/celestiaorg/celestia-node/node"
	"github.com/celestiaorg/test-infra/testkit/appkit"
	"github.com/celestiaorg/test-infra/testkit/nodekit"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/testground/sdk-go/network"
	"github.com/testground/sdk-go/run"
	"github.com/testground/sdk-go/runtime"
	"github.com/testground/sdk-go/sync"
)

var testcases = map[string]interface{}{
	"capp-3":   run.InitializedTestCaseFn(runSync),
	"init-val": run.InitializedTestCaseFn(initVal),
}

func main() {
	run.InvokeMap(testcases)
}

type AppId struct {
	ID int
	IP net.IP
}

type BridgeId struct {
	ID          int
	Maddr       string
	TrustedHash string
	Amount      int
}

func runSync(runenv *runtime.RunEnv, initCtx *run.InitContext) error {
	ctx := context.Background()
	client := initCtx.SyncClient
	netclient := network.NewClient(client, runenv)

	netclient.MustWaitNetworkInitialized(ctx)

	config := network.Config{
		// Control the "default" network. At the moment, this is the only network.
		Network: "default",

		// Enable this network. Setting this to false will disconnect this test
		// instance from this network. You probably don't want to do that.
		Enable: true,

		// Set the traffic shaping characteristics.
		Default: network.LinkShape{
			Latency:   100 * time.Millisecond,
			Bandwidth: 1 << 20, // 1Mib
		},

		// Set what state the sidecar should signal back to you when it's done.
		CallbackState: "network-configured",
		RoutingPolicy: network.AllowAll,
	}

	topic := sync.NewTopic("ip-allocation", "")
	seq := client.MustPublish(ctx, topic, "")

	config.IPv4 = runenv.TestSubnet
	// Use the sequence number to fill in the last two octets.
	//
	// NOTE: Be careful not to modify the IP from `runenv.TestSubnet`.
	// That could trigger undefined behavior.
	ipC := byte((seq >> 8) + 1)
	ipD := byte(seq)
	config.IPv4.IP = append(config.IPv4.IP[0:2:2], ipC, ipD)

	err := netclient.ConfigureNetwork(ctx, &config)
	if err != nil {
		runenv.RecordCrash(err)
		return err
	}

	appt := sync.NewTopic("app-id", &AppId{})
	bridget := sync.NewTopic("bridge-id", &BridgeId{})
	stateDone := sync.State("done")
	if runenv.TestGroupID == "app" {
		home := runenv.StringParam(fmt.Sprintf("app%d", initCtx.GroupSeq))

		fmt.Println(home)
		cmd := appkit.NewRootCmd()

		nodeId, err := appkit.GetNodeId(cmd, home)
		if err != nil {
			runenv.RecordCrash(err)
			return err
		}

		valt := sync.NewTopic("validator-info", &appkit.ValidatorNode{})
		client.Publish(ctx, valt, &appkit.ValidatorNode{nodeId, config.IPv4.IP})

		rdySt := sync.State("appReady")
		appseq := client.MustSignalEntry(ctx, rdySt)
		client.Publish(ctx, appt, &AppId{int(appseq), config.IPv4.IP})
		<-client.MustBarrier(ctx, rdySt, runenv.TestGroupInstanceCount).C

		valCh := make(chan *appkit.ValidatorNode)
		client.Subscribe(ctx, valt, valCh)

		var persPeers []string
		for i := 0; i < runenv.TestGroupInstanceCount; i++ {
			val := <-valCh
			runenv.RecordMessage("Validator Received: %s, %s", val.IP, val.PubKey)
			if !val.IP.Equal(config.IPv4.IP) {
				persPeers = append(persPeers, fmt.Sprintf("%s@%s", val.PubKey, val.IP.To4().String()))
			}
		}

		configPath := filepath.Join(home, "config", "config.toml")
		err = appkit.AddPersistentPeers(configPath, persPeers)
		if err != nil {
			return err
		}

		go appkit.StartNode(cmd, home)
		client.MustSignalAndWait(ctx, stateDone, int(initCtx.GlobalSeq))
		err = syscall.Kill(syscall.Getpid(), syscall.SIGINT)
		if err != nil {
			return err
		}

		return nil

	} else if runenv.TestGroupID == "bridge" {
		os.Setenv("GOLOG_OUTPUT", "stdout")

		time.Sleep(8 * time.Second)
		level, err := logging.LevelFromString("INFO")
		if err != nil {
			return err
		}
		logs.SetAllLoggers(level)
		appIPCh := make(chan *AppId)
		client.Subscribe(ctx, appt, appIPCh)
		for i := 1; i <= runenv.TestGroupInstanceCount; i++ {
			appIP := <-appIPCh
			if appIP.ID == int(initCtx.GroupSeq) {
				h, err := appkit.GetBlockHashByHeight(appIP.IP, 1)
				if err != nil {
					return err
				}
				runenv.RecordMessage("Block#1 Hash: %s", h)

				ndhome := fmt.Sprintf("/.celestia-bridge-%d", initCtx.GroupSeq)
				rc := fmt.Sprintf("%s:26657", appIP.IP.To4().String())
				runenv.RecordMessage(rc)
				nd, err := nodekit.NewNode(ndhome, node.Bridge, config.IPv4.IP, node.WithTrustedHash(h), node.WithRemoteCore("tcp", rc))
				if err != nil {
					return err
				}

				ndCtx := context.Background()
				nd.Start(ndCtx)
				if err != nil {
					return err
				}

				eh, err := nd.HeaderServ.GetByHeight(ndCtx, uint64(4))
				if err != nil {
					return err
				}

				runenv.RecordMessage("Reached Block#4 contains Hash: %s", eh.Commit.BlockID.Hash.String())

				//create a new subscription to publish bridge's multiaddress to full/light nodes
				addrs, err := peer.AddrInfoToP2pAddrs(host.InfoFromHost(nd.Host))
				if err != nil {
					return err
				}

				runenv.RecordMessage("Publishing bridgeID %d", int(initCtx.GroupSeq))
				runenv.RecordMessage("Publishing bridgeID Addr %s", addrs[0].String())

				bseq, err := client.Publish(ctx, bridget, &BridgeId{int(initCtx.GroupSeq), addrs[0].String(), h, runenv.TestGroupInstanceCount})
				if err != nil {
					return err
				}

				runenv.RecordMessage("%d published bridge id", int(bseq))
				client.MustSignalAndWait(ctx, stateDone, int(initCtx.GlobalSeq))

				err = nd.Stop(ndCtx)
				if err != nil {
					return err
				}
				return nil
			}
		}
	} else if runenv.TestGroupID == "full" {
		os.Setenv("GOLOG_OUTPUT", "stdout")
		level, err := logging.LevelFromString("INFO")
		if err != nil {
			return err
		}
		logs.SetAllLoggers(level)

		bridgeCh := make(chan *BridgeId)
		sub, err := client.Subscribe(ctx, bridget, bridgeCh)
		if err != nil {
			return err
		}

		for {
			select {
			case <-sub.Done():
				return fmt.Errorf("nodeId hasn't received")
			case bridge := <-bridgeCh:
				if int(initCtx.GroupSeq) == bridge.ID {
					ndhome := fmt.Sprintf("/.celestia-full-%d", initCtx.GroupSeq)
					runenv.RecordMessage(ndhome)
					nd, err := nodekit.NewNode(ndhome, node.Full, config.IPv4.IP, node.WithTrustedHash(bridge.TrustedHash), node.WithTrustedPeers(bridge.Maddr))
					if err != nil {
						return err
					}
					ndCtx := context.Background()
					err = nd.Start(ndCtx)
					if err != nil {
						return err
					}

					eh, err := nd.HeaderServ.GetByHeight(ndCtx, uint64(9))
					if err != nil {
						return err
					}
					runenv.RecordMessage("Reached Block#9 contains Hash: %s", eh.Commit.BlockID.Hash.String())

					err = nd.Stop(ndCtx)
					if err != nil {
						return err
					}
					client.MustSignalAndWait(ctx, stateDone, int(initCtx.GlobalSeq))
					return nil
				}
			}
		}
	} else if runenv.TestGroupID == "light" {
		os.Setenv("GOLOG_OUTPUT", "stdout")

		level, err := logging.LevelFromString("INFO")
		if err != nil {
			return err
		}
		logs.SetAllLoggers(level)
		bridgeCh := make(chan *BridgeId)
		sub, err := client.Subscribe(ctx, bridget, bridgeCh)
		if err != nil {
			return err
		}

		for {

			select {
			case <-sub.Done():
				return fmt.Errorf("nodeId hasn't received")
			case bridge := <-bridgeCh:

				//we receive bridgeIDs that contain the ID of bridge and the total amount of bridges
				//we need to assign light nodes 30/30/30 per each bridge
				if int(initCtx.GroupSeq)%bridge.Amount == bridge.ID%bridge.Amount {
					ndhome := fmt.Sprintf("/.celestia-light-%d", int(initCtx.GroupSeq))
					runenv.RecordMessage(ndhome)
					nd, err := nodekit.NewNode(ndhome, node.Light, config.IPv4.IP, node.WithTrustedHash(bridge.TrustedHash), node.WithTrustedPeers(bridge.Maddr))
					if err != nil {
						return err
					}
					ndCtx := context.Background()
					err = nd.Start(ndCtx)
					if err != nil {
						return err
					}

					eh, err := nd.HeaderServ.GetByHeight(ndCtx, uint64(9))
					if err != nil {
						return err
					}

					runenv.RecordMessage("Reached Block#9 contains Hash: %s", eh.Commit.BlockID.Hash.String())
					runenv.RecordSuccess()

					err = nd.Stop(ndCtx)
					if err != nil {
						return err
					}
					client.MustSignalAndWait(ctx, stateDone, int(initCtx.GlobalSeq))
					return nil
				}
			}
		}
	}
	return nil
}

func initVal(runenv *runtime.RunEnv, initCtx *run.InitContext) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*1)
	defer cancel()
	client := initCtx.SyncClient
	netclient := network.NewClient(client, runenv)

	netclient.MustWaitNetworkInitialized(ctx)

	config := network.Config{
		// Control the "default" network. At the moment, this is the only network.
		Network: "default",

		// Enable this network. Setting this to false will disconnect this test
		// instance from this network. You probably don't want to do that.
		Enable: true,

		// Set the traffic shaping characteristics.
		Default: network.LinkShape{
			Latency:   100 * time.Millisecond,
			Bandwidth: 1 << 20, // 1Mib
		},

		// Set what state the sidecar should signal back to you when it's done.
		CallbackState: "network-configured",
		RoutingPolicy: network.AllowAll,
	}

	topic := sync.NewTopic("ip-allocation", "")
	seq := client.MustPublish(ctx, topic, "")

	config.IPv4 = runenv.TestSubnet

	ipC := byte((seq >> 8) + 1)
	ipD := byte(seq)
	config.IPv4.IP = append(config.IPv4.IP[0:2:2], ipC, ipD)

	err := netclient.ConfigureNetwork(ctx, &config)
	if err != nil {
		runenv.RecordCrash(err)
		return err
	}

	// init the chain
	home := fmt.Sprintf("/.celestia-app-%d", initCtx.GroupSeq)
	fmt.Println(home)

	cmd := appkit.NewRootCmd()

	addrt := sync.NewTopic("account-address", "")

	output, err := appkit.CreateKey(cmd, "xm1", "test", home)
	if err != nil {
		return err
	}

	client.Publish(ctx, addrt, output)

	initgen := sync.NewTopic("init-gen", []byte(nil))

	if runenv.TestGroupID == "orc" {
		addrch := make(chan string)
		client.Subscribe(ctx, addrt, addrch)

		var accounts []string
		for i := 0; i < runenv.TestInstanceCount; i++ {
			addr := <-addrch
			fmt.Println("Received address: ", addr)
			accounts = append(accounts, addr)
		}

		fmt.Println(runenv.TestInstanceCount)
		_, err = appkit.InitChain(cmd, "kek", "tia-test", home)
		if err != nil {
			return err
		}

		for _, v := range accounts {
			out, err := appkit.AddGenAccount(cmd, v, "1000000000000000utia", home)
			if err != nil {
				return err
			}
			fmt.Println(out)
		}

		gen, err := os.Open(fmt.Sprintf("%s/config/genesis.json", home))
		if err != nil {
			return err
		}

		bt, err := ioutil.ReadAll(gen)
		if err != nil {
			return err
		}

		var res map[string]interface{}
		json.Unmarshal([]byte(bt), &res)

		client.Publish(ctx, initgen, bt)

		fmt.Println("finish orc")

	}

	if runenv.TestGroupID == "rov" {
		ingench := make(chan []byte)
		client.Subscribe(ctx, initgen, ingench)

		ingen := <-ingench
		// fmt.Println(ingen)

		var res map[string]interface{}
		json.Unmarshal([]byte(ingen), &res)

		fmt.Println(res)
		err := ioutil.WriteFile(fmt.Sprintf("%s/config/genesis.json", home), ingen, 0777)
		if err != nil {
			return err
		}
		fmt.Println("rov finish")
	}

	output, err = appkit.SignGenTx(cmd, "xm1", "5000000000utia", "test", "tia-test", home)
	if err != nil {
		return err
	}

	// gent := sync.NewTopic("gentx", []byte(nil))

	if runenv.TestGroupID == "rov" {
		fs, err := os.ReadDir(fmt.Sprintf("%s/config/gentx", home))
		if err != nil {
			return err
		}

		for _, f := range fs {
			gentx, err := os.Open(fmt.Sprintf("%s/config/gentx/%s", home, f.Name()))
			if err != nil {
				return err
			}

			bt, err := ioutil.ReadAll(gentx)
			if err != nil {
				return err
			}

			client.Publish(ctx, initgen, bt)

			fmt.Println("rov sent gentx")
		}
	} else {
		gentch := make(chan []byte)
		client.Subscribe(ctx, initgen, gentch)

		for i := 0; i < runenv.TestInstanceCount; i++ {
			gentx := <-gentch
			if i != 0 {
				var res map[string]interface{}
				json.Unmarshal([]byte(gentx), &res)
				fmt.Println("----------------------------------------------------")
				fmt.Println(res)
				fmt.Println("----------------------------------------------------")
				err := ioutil.WriteFile(fmt.Sprintf("%s/config/gentx/%d.json", home, i), gentx, 0777)
				if err != nil {
					return err
				}
			}
		}

		fs, err := os.ReadDir(fmt.Sprintf("%s/config/gentx", home))
		if err != nil {
			return err
		}
		fmt.Println("|||||||||||||||||||||||")
		for _, f := range fs {
			fmt.Println(f.Name())
		}

		fmt.Println("orc got all gentxs")

		output, err = appkit.CollectGenTxs(cmd, home)
		if err != nil {
			return err
		}

		fmt.Println(output)
	}

	fmt.Println("finish line")
	return nil
}
