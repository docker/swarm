#!/usr/bin/env bats

load ../helpers

function teardown() {
	swarm_manage_cleanup
	stop_docker
}

@test "docker network ls" {
	start_docker 2
	swarm_manage

	run docker_swarm network ls
	[ "${#lines[@]}" -eq 7 ]
}

@test "docker network inspect" {
	start_docker_with_busybox 2
	swarm_manage

	# run
	docker_swarm run -d -e constraint:node==node-0 busybox sleep 100

	run docker_swarm network inspect bridge
	[ "$status" -ne 0 ]

	run docker_swarm network inspect node-0/bridge
	[ "${#lines[@]}" -eq 13 ]
}

@test "docker volume create" {
skip
	start_docker 2
	swarm_manage

	run docker_swarm volume ls
	[ "${#lines[@]}" -eq 1 ]

	docker_swarm volume create --name=test_volume
	run docker_swarm volume
	[ "${#lines[@]}" -eq 3 ]

	docker_swarm run -d -v=/tmp busybox true
	run docker_swarm volume
	[ "${#lines[@]}" -eq 4 ]
}

@test "docker volume rm" {
skip
	start_docker_with_busybox 2
	swarm_manage

	run docker_swarm volume rm test_volume
	[ "$status" -ne 0 ]

	docker_swarm run -d --name=test_container -v=/tmp busybox true
	
	run docker_swarm volume ls -q
	volume=${output}
	[ "${#lines[@]}" -eq 1 ]

	run docker_swarm volume rm $volume
	[ "$status" -ne 0 ]

	docker_swarm rm test_container
	
	run docker_swarm volume rm $volume
	[ "$status" -eq 0 ]
	[ "${#lines[@]}" -eq 1 ]
	
	run docker_swarm volume
	echo $output
	[ "${#lines[@]}" -eq 1 ]
}
