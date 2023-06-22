.ONESHELL: #

SHELL=/bin/bash
.SHELLFLAGS += -e
PROJECTNAME=$(shell basename "$(PWD)")
DIR_FULLPATH=$(shell pwd)
TGPATH=
ifeq (${TGPATH},)
	TGPATH := /usr/local/testground
endif

## help: Get more info on make commands.
help: Makefile
	@echo " Choose a command run in "$(PROJECTNAME)":"
	@sed -n 's/^##//p' $< | column -t -s ':' |  sed -e 's/^/ /'
.PHONY: help

## check-git: Check if git is installed on the machine
check-git:
ifeq (,$(shell which git))	
	@echo "git is not installed, you must install it first."
	exit 1;
endif
.PHONY: check-git

## check-go: Check if go is installed on the machine
check-go:
ifeq (,$(shell which go))	
	@echo "go is not installed, you must install it first."
	exit 1;
endif
.PHONY: check-go

## check-gcc: Check if gcc is installed on the machine
check-gcc:
ifeq (,$(shell which gcc))	
	@echo "gcc is not installed, you must install it first."
	exit 1;
endif
.PHONY: check-go

## check-docker: Check if docker is installed on the machine
check-docker:
ifeq (,$(shell which docker))	
	@echo "docker is not installed, you must install docker first."
	exit 1;
else
ifeq (/snap/bin/docker,$(shell which docker))
	@echo "You have docker installed through snap. This is problematic, snap won't let `install-tg` do its job properly with Docker. Aborting"
	exit 1;
endif
endif
.PHONY: check-go

## check-docker: Check if docker is installed on the machine
check-docker-compose:
ifeq (,$(shell which docker-compose))	
	@echo "docker-compose is not installed, you must install it first."
	exit 1;
endif
.PHONY: check-go

## install-tg: Install testground into the $TGPATH.
install-tg: check-git check-go check-gcc check-docker
	@echo "Do you want to install to ${TGPATH}? (y/n):"
	@read line; if [ $$line = "n" ]; then echo "Please retry with TGPATH set to your desired installation path."; exit 1 ; fi
	@echo "Installing testground to the following path: ${TGPATH}"
	@git clone https://github.com/testground/testground.git ${TGPATH}
	@cd ${TGPATH}
	echo $(pwd)
	@make install
	@echo "Done."
.PHONY: install-tg

## check-testplan-arg: Check if TSETPLAN env var was set
check-testplan-arg:
ifeq (,${TESTPLAN})
	@printf "You must specify a testplan, example:\n\t make COMMAND TESTPLAN=local-docker\n\n"
	exit 1
endif
.PHONY: check-testplan-arg

## check-runner-arg: Check if RUNNER env var was set
check-runner-arg:
ifeq (,${RUNNER})
	@printf "You must specify which runner you want to use, example:\n\t make COMMAND RUNNER=local-docker \n\n"
	exit 1
endif
.PHONY: check-runner-arg

## check-composition-arg: check if composition env var was set
check-composition-arg:
ifeq (,${COMPOSITION})
	@printf "you must specify a testplan, example:\n\t make COMMAND COMPOSITION=pdf-8\n\n"
	exit 1
endif
.PHONY: check-composition-arg

## check-composition-arg: check if composition env var was provided
check-name-arg:
ifeq (,${NAME})
	@printf "you must specify a testplan, example:\n\t make COMMAND NAME=celestia\n\n"
	exit 1
endif
.phony: check-name-arg

## check-podname-arg: check if podname env var was set
check-podname-arg:
ifeq (,${POD_NAME})
	@printf "you must specify a podname, example:\n\t make COMMAND POD_NAME=influxdb\n\n"
	exit 1
endif
.PHONY: check-podname-arg

## check-square-size-arg: check if square size env var was set
check-square-size-arg:
ifeq (,${SQUARE_SIZE})
	@printf "you must specify a square size, example:\n\t make COMMAND SQUARE_SIZE=128\n\n"
	exit 1
endif
.phony: check-square-size-arg

## check-getter-arg: check if getter env var was set
check-getter-arg:
ifeq (,${GETTER})
	@printf "you must specify a getter, example:\n\t make COMMAND GETTER=ipld\n\n"
	exit 1
endif
.phony: check-getter-arg

## check-pod-port-arg: check if pod port env var was set
check-pod-port-arg:
ifeq (,${POD_PORT})
	@printf "you must specify a pod port, example:\n\t make COMMAND POD_PORT=3000\n\n"
	exit 1
endif
.phony: check-pod-port-arg

## check-local-port-arg: check if local port env var was set
check-local-port-arg:
ifeq (,${LOCAL_PORT})
	@printf "you must specify a local port, example:\n\t make COMMAND LOCAL_PORT=3000\n\n"
	exit 1
endif
.phony: check-local-port-arg


## tg-start: Start the testground deamon
tg-start:
	testground daemon
.PHONY: tg-start

## tg-create-testplan: Create test plan under ./plans/ of this repository
# tg-create-testplan: check-name-arg
# 	TESTGROUND_HOME=${DIR_FULLPATH} testground plan create --plan=${NAME}
# 	@rm -rf ./data ./sdks
# 	@mkdir ./docs/test-plans/${NAME}
# 	@cp ./docs/test-plans/tp-template.md ./docs/test-plans/${NAME}/${NAME}.md
# .PHONY: tg-create-testplan

## tg-import-testplan Import testplan to TESTGROUND_HOME
# tg-import-testplan: check-testplan-arg check-name-arg
# 	testground plan import --from ./plans/${TESTPLAN} --name ${NAME}	
# .PHONY: tg-import-testplan

## tg-run-composition: runs a specific composition by name given a testplan and a runner
tg-run-composition: check-testplan-arg check-runner-arg check-composition-arg
	@testground run composition \
		-f compositions/${RUNNER}/${TESTPLAN}/${COMPOSITION}.toml \
		--wait
.PHONY: tg-run-testplan

## tg-run-composition: runs a specific composition by name given a testplan and a runner
tg-run-composition-no-wait: check-testplan-arg check-runner-arg check-composition-arg
	@testground run composition \
		-f compositions/${RUNNER}/${TESTPLAN}/${COMPOSITION}.toml 
.PHONY: tg-run-testplan

## telemetry-infra-up: launches the telemetry infrastructure up
telemetry-infra-up: check-docker check-docker-compose
	PWD="${DIR_FULLPATH}/docker/local-telemetry" docker-compose -f ./docker/local-telemetry/docker-compose.yml up
.PHONY: telemetry-infra-up

## telemetry-infra-up: launches the telemetry infrastructure up
telemetry-infra-down: check-docker check-docker-compose
	PWD="${DIR_FULLPATH}/docker/local-telemetry" docker-compose -f ./docker/local-telemetry/docker-compose.yml down
.PHONY: telemetry-infra-down

## check-docker: Check if docker is installed on the machine
check-kubectl:
ifeq (,$(shell which kubectl))	
	@echo "kubectl is not installed, you must install kubectl first."
	exit 1;
endif
.PHONY: check-go

## port-forward: port forwards a k8s pod to be used locally with local grafana instances
port-forward: check-kubectl check-podname-arg check-pod-port-arg check-local-port-arg
	kubectl port-forward ${POD_NAME} --address 0.0.0.0 ${LOCAL_PORT}:${POD_PORT}
.PHONY: port-forward

## block-sync-latest: run block-sync latest ipld-only composition
block-sync: check-getter-arg check-square-size-arg
ifeq (,${VARIATION})
	@printf "you must specify a block sync variation (latest|historical), example:\n\t make block-sync VARIATION=latest\n\n"
	exit 1
endif
	make tg-run-composition-no-wait \
		RUNNER=cluster-k8s \
		TESTPLAN=block-sync \
		COMPOSITION=${VARIATION}/${SQUARE_SIZE}-square-size/1-3-32-${GETTER}
.PHONY: block-sync-latest-ipld

## tail-pod-logs: tail pod logs that are persisted to the the file system note that these logs are different from the ones that received from `kubectl logs -f <pod-name>`
tail-pod-logs: check-podname-arg
	kubectl exec -i -t ${POD_NAME} tail /var/log/node.log
.PHONY: tail-pod-logs