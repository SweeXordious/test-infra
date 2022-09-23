package main

import (
	"github.com/celestiaorg/test-infra/tests"
	"github.com/testground/sdk-go/run"
)

var testcases = map[string]interface{}{
	"001-val-large-txs":  run.InitializedTestCaseFn(tests.ValSubmitLargeTxs),
	"002-da-sync":        run.InitializedTestCaseFn(tests.SyncNodes),
	"003-full-sync-past": run.InitializedTestCaseFn(tests.FullSyncPast),
}

func main() {
	run.InvokeMap(testcases)
}
