#!/usr/bin/env bats

load helpers

# Address on which Zookeeper will listen (random port between 7000 and 8000).
ZK_HOST=127.0.0.1:$(( ( RANDOM % 1000 )  + 7000 ))

# Container name for integration test
ZK_CONTAINER_NAME=swarm_integration_zk

function start_zk() {
	docker_host run --name $ZK_CONTAINER_NAME -p $ZK_HOST:2181 -d jplock/zookeeper
}

function stop_zk() {
	docker_host rm -f -v $ZK_CONTAINER_NAME
}

function setup() {
	start_zk
}

function teardown() {
	swarm_manage_cleanup
	swarm_join_cleanup
	stop_docker
	stop_zk
}

@test "zookeeper discovery" {
	# Start 2 engines and make them join the cluster.
	start_docker 2
	swarm_join "zk://${ZK_HOST}/test"

	# Start a manager and ensure it sees all the engines.
	swarm_manage "zk://${ZK_HOST}/test"
	check_swarm_nodes

	# Add another engine to the cluster and make sure it's picked up by swarm.
	start_docker 1
	swarm_join "zk://${ZK_HOST}/test"
	retry 30 1 check_swarm_nodes
}
