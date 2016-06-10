#! /bin/bash

. ./config.sh

# Skip if it is run on non-AWS machine
[ -z "$AWS" ] && exit 0

UNIVERSE=10.32.0.0/12
SUBNET=10.32.42.0/24
CIDR1=10.32.0.0/14
CIDR2=10.36.0.0/14
CIDR3=10.40.0.0/14
CIDR4=10.44.0.0/14

INSTANCE_ID_CMD="curl -s -L http://169.254.169.254/latest/meta-data/instance-id"

# TODO(mp) Detect by using instance id instead!
routetableid() {
    host=$1
    json=$(mktemp json.XXXXXXXXXX)
    aws ec2 describe-instances                                      \
        --filters "Name=instance-state-name,Values=pending,running" \
                  "Name=tag:weavenet_ci,Values=true"                \
                  "Name=tag:Name,Values=$host" > $json
    vpcid=$(jq -r ".Reservations[0].Instances[0].NetworkInterfaces[0].VpcId" $json)
    aws ec2 describe-route-tables                                   \
        --filters "Name=vpc-id,Values=$vpcid" > $json
    jq -r ".RouteTables[0].RouteTableId" $json
    rm $json
}

cleanup_routetable() {
    id=$1
    json=$(mktemp json.XXXXXXXXXX)
    echo "Cleaning up routes"
    aws ec2 describe-route-tables --route-table-ids $id > $json
    cidrs=$(jq -r ".RouteTables[0].Routes[] | select(has(\"NetworkInterfaceId\")) |
                    .DestinationCidrBlock" $json)
    for cidr in $cidrs; do
        echo "Removing $cidr route"
        aws ec2 delete-route                \
            --route-table-id $id            \
            --destination-cidr-block $cidr
    done
    rm $json
}

route_exists() {
    rt_id=$1
    dst_cidr=$2
    instance_id=$3
    q=".RouteTables[].Routes[] | select (.DestinationCidrBlock == \"$dst_cidr\") |
        select (.InstanceId == \"$instance_id\")"
    aws ec2 describe-route-tables --route-table-ids $rt_id |
        jq -e -r "$q" > /dev/null
}

route_not_exist() {
    rt_id=$1
    dst_cidr=$2
    q=".RouteTables[].Routes[] | select (.DestinationCidrBlock == \"$dst_cidr\")"
    aws ec2 describe-route-tables --route-table-ids $rt_id |
        jq -e -r "$q" > /dev/null
    [ $? -ne 0 ] || return 1
}

no_fastdp() {
    weave_on $1 report -f "{{.Router.Interface}}" | grep -q -v "datapath"
}

start_suite "AWS VPC"

INSTANCE1=$($SSH $HOST1 $INSTANCE_ID_CMD)
INSTANCE2=$($SSH $HOST2 $INSTANCE_ID_CMD)
INSTANCE3=$($SSH $HOST3 $INSTANCE_ID_CMD)

VPC_ROUTE_TABLE_ID=$(routetableid $HOST1)
cleanup_routetable $VPC_ROUTE_TABLE_ID

echo "starting weave"

weave_on $HOST1 launch --log-level=debug --ipalloc-range $UNIVERSE --awsvpc
weave_on $HOST2 launch --log-level=debug --ipalloc-range $UNIVERSE --awsvpc $HOST1
weave_on $HOST3 launch --log-level=debug --ipalloc-range $UNIVERSE --awsvpc $HOST1

echo "starting containers"

start_container $HOST1 --name=c1
start_container $HOST2 --name=c4
start_container $HOST1 --name=c2
proxy_start_container $HOST1 -di --name=c3
start_container $HOST3 --name=c5

assert_raises "route_exists $VPC_ROUTE_TABLE_ID $CIDR1 $INSTANCE1"
assert_raises "route_exists $VPC_ROUTE_TABLE_ID $CIDR2 $INSTANCE3"
assert_raises "route_exists $VPC_ROUTE_TABLE_ID $CIDR3 $INSTANCE1"
assert_raises "route_exists $VPC_ROUTE_TABLE_ID $CIDR4 $INSTANCE2"

# Starting container within non-default subnet should fail
assert_raises "proxy_start_container $HOST1 --name=c6 -e WEAVE_CIDR=net:$SUBNET" 1

# Check that we do not use fastdp
assert_raises "no_fastdp $HOST1"
assert_raises "no_fastdp $HOST2"
assert_raises "no_fastdp $HOST3"

assert_raises "exec_on $HOST1 c1 $PING c2"
assert_raises "exec_on $HOST1 c1 $PING c4"
assert_raises "exec_on $HOST2 c4 $PING c1"
assert_raises "exec_on $HOST2 c4 $PING c3"
assert_raises "exec_on $HOST1 c1 $PING c5"
assert_raises "exec_on $HOST2 c4 $PING c5"

weave_on $HOST2 stop
# stopping should not remove the entries
assert_raises "route_exists $VPC_ROUTE_TABLE_ID $CIDR4 $INSTANCE2"

weave_on $HOST2 launch --log-level=debug --ipalloc-range $UNIVERSE --awsvpc $HOST1

weave_on $HOST1 reset
PEER3=$(weave_on $HOST3 report -f '{{.Router.Name}}')
weave_on $HOST3 stop
weave_on $HOST2 rmpeer $PEER3

## host1 transferred previously owned ranges to host2 and host2 took over host2 ranges
assert_raises "route_not_exist $VPC_ROUTE_TABLE_ID $CIDR1"
assert_raises "route_not_exist $VPC_ROUTE_TABLE_ID $CIDR2"
assert_raises "route_not_exist $VPC_ROUTE_TABLE_ID $CIDR3"
assert_raises "route_not_exist $VPC_ROUTE_TABLE_ID $CIDR4"
assert_raises "route_exists $VPC_ROUTE_TABLE_ID $UNIVERSE $INSTANCE2"

cleanup_routetable $VPC_ROUTE_TABLE_ID

end_suite
