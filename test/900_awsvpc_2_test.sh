#! /bin/bash

. ./config.sh

# Skip if it is run on non-AWS machine
[ -z "$AWS" ] && exit 0

UNIVERSE=10.32.0.0/12
CIDR1=10.32.0.0/13
CIDR2=10.40.0.0/14
CIDR3=10.44.0.0/14

INSTANCE_ID_CMD="curl -s -L http://169.254.169.254/latest/meta-data/instance-id"

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
    aws ec2 describe-route-tables --route-table-ids $rtid |
        jq -e -r "$q" > /dev/null
}

route_not_exist() {
    rt_id=$1
    dst_cidr=$2
    q=".RouteTables[].Routes[] | select (.DestinationCidrBlock == \"$dst_cidr\")"
    [[ ! $(aws ec2 describe-route-tables --route-table-ids $rtid |
        jq -e -r "$q" > /dev/null) ]]
}

no_fastdp() {
    weave_on $1 report -f "{{.Router.Interface}}" | grep -q -v "datapath"
}

start_suite "AWS VPC"

INSTANCE1=$($SSH $HOST1 $INSTANCE_ID_CMD)
INSTANCE2=$($SSH $HOST2 $INSTANCE_ID_CMD)

VPC_ROUTE_TABLE_ID=$(routetableid $HOST1)
cleanup_routetable $VPC_ROUTE_TABLE_ID

echo "starting weave"

weave_on $HOST1 launch --log-level=debug --ipalloc-range $UNIVERSE --awsvpc
weave_on $HOST2 launch --log-level=debug --ipalloc-range $UNIVERSE --awsvpc $HOST1

echo "starting containers"

start_container $HOST1 --name=c1
start_container $HOST2 --name=c3
start_container $HOST1 --name=c2

assert_raises "route_exists $VPC_ROUTE_TABLE_ID $CIDR1 $INSTANCE1"
assert_raises "route_exists $VPC_ROUTE_TABLE_ID $CIDR2 $INSTANCE1"
assert_raises "route_exists $VPC_ROUTE_TABLE_ID $CIDR3 $INSTANCE2"

# Check that we do not use fastdp
assert_raises "no_fastdp $HOST1"
assert_raises "no_fastdp $HOST2"

assert_raises "exec_on $HOST1 c1 $PING c2"
assert_raises "exec_on $HOST1 c1 $PING c3"
assert_raises "exec_on $HOST2 c3 $PING c1"

weave_on $HOST2 stop
# stopping should not remove the entries
assert_raises "route_exists $VPC_ROUTE_TABLE_ID $CIDR3 $INSTANCE2"

weave_on $HOST2 launch --log-level=debug --ipalloc-range $UNIVERSE --awsvpc $HOST1

weave_on $HOST1 reset
## host1 has transferred previously owned ranges to host2
assert_raises "route_not_exist $VPC_ROUTE_TABLE_ID $CIDR1"
assert_raises "route_not_exist $VPC_ROUTE_TABLE_ID $CIDR2"
assert_raises "route_not_exist $VPC_ROUTE_TABLE_ID $CIDR3"
assert_raises "route_exists $VPC_ROUTE_TABLE_ID $UNIVERSE $INSTANCE2"

cleanup_routetable $VPC_ROUTE_TABLE_ID

end_suite
