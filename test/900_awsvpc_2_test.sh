#! /bin/bash

. ./config.sh

# Skip if it is intended to run on non-AWS machine
[ -z "$AWS" ] && exit 0

CIDR1=10.32.0.0/13
CIDR2=10.40.0.0/13
UNIVERSE=10.32.0.0/12

function routetableid {
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

function cleanup_routetable {
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

function route_exists {
    aws ec2 describe-route-tables --route-table-ids $1 |
    jq -e -r ".RouteTables[].Routes[] | select (.DestinationCidrBlock == \"$2\")" > /dev/null
    [ $? -eq 0 ]
}

function no_fastdp {
    weave_on $1 report -f "{{.Router.Interface}}" | grep -q -v "datapath"
}

start_suite "AWS VPC"

VPC_ROUTE_TABLE_ID=$(routetableid $HOST1)
cleanup_routetable $VPC_ROUTE_TABLE_ID

weave_on $HOST1 launch                              \
        --ipalloc-range $UNIVERSE                   \
        --awsvpc
weave_on $HOST2 launch                              \
        --ipalloc-range $UNIVERSE                   \
        --awsvpc                                    \
        $HOST1

start_container $HOST1 --name=c1
start_container $HOST2 --name=c2

assert_raises "route_exists $VPC_ROUTE_TABLE_ID $CIDR1"
assert_raises "route_exists $VPC_ROUTE_TABLE_ID $CIDR2"

# Check that we do not use fastdp
assert_raises "no_fastdp $HOST1"
assert_raises "no_fastdp $HOST2"

assert_raises "exec_on $HOST1 c1 $PING c2"
assert_raises "exec_on $HOST2 c2 $PING c1"

weave_on $HOST1 reset
## host1 has transferred previously owned ranges to host2
assert_raises "route_exists $VPC_ROUTE_TABLE_ID $CIDR1" 1
assert_raises "route_exists $VPC_ROUTE_TABLE_ID $CIDR2" 1
assert_raises "route_exists $VPC_ROUTE_TABLE_ID $UNIVERSE"

cleanup_routetable $VPC_ROUTE_TABLE_ID

end_suite
