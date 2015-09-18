#! /bin/bash

. ./config.sh

start_suite "Abuse of 'start' operation"

weave_on $HOST1 launch

proxy_start_container $HOST1 --name=c1
proxy docker_on $HOST1 create --name=c2 $SMALL_IMAGE grep "^1$" /sys/class/net/ethwe/carrier
# Now start c2 with a sneaky HostConfig
curl -X POST -H Content-Type:application/json -d '{"NetworkMode": "container:c1"}' http://$HOST1:12375/v1.20/containers/c2/start
docker_bridge_ip=$(weave_on $HOST1 docker-bridge-ip)
assert "docker_on $HOST1 inspect -f '{{.State.Running}} {{.State.ExitCode}} {{.HostConfig.Dns}}' c2" "false 0 [$docker_bridge_ip]"

# Now start c3 with a mostly null HostConfig
proxy docker_on $HOST1 create --name=c3 -v /tmp:/hosttmp $SMALL_IMAGE grep "^1$" /sys/class/net/ethwe/carrier
curl -X POST -H Content-Type:application/json -d '{"Binds":[],"ContainerIDFile":"","LxcConf":null,"Memory":0,"MemorySwap":0,"CpuShares":0,"CpuPeriod":0,"CpusetCpus":"","CpusetMems":"","CpuQuota":0,"BlkioWeight":0,"OomKillDisable":false,"Privileged":false,"PortBindings":null,"Links":[],"PublishAllPorts":false,"Dns":null,"DnsSearch":null,"ExtraHosts":null,"VolumesFrom":null,"Devices":null,"NetworkMode":"","IpcMode":"","PidMode":"","UTSMode":"","CapAdd":null,"CapDrop":null,"RestartPolicy":{"Name":"","MaximumRetryCount":0},"SecurityOpt":null,"ReadonlyRootfs":false,"Ulimits":null,"LogConfig":{"type":"","config":null},"CgroupParent":""}' http://$HOST1:12375/v1.20/containers/c3/start
assert "docker_on $HOST1 inspect -f '{{.State.Running}} {{.State.ExitCode}} {{.HostConfig.Dns}} {{index .HostConfig.Binds 0}}' c3" "false 0 [$docker_bridge_ip] /tmp:/hosttmp"

end_suite
