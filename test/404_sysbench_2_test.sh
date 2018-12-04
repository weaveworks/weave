#! /bin/bash

. "$(dirname "$0")/config.sh"

start_suite "Sysbench"

TABLE_COUNT=1
TABLE_SIZE=10
NUM_THREADS=1

weave_on $HOST1 launch
weave_on $HOST2 launch $HOST1
weave_on $HOST2 prime

$SSH $HOST1 tee /tmp/my.cnf >/dev/null <<EOF
[mysqld]
datadir=/var/lib/mysql
sql_mode=NO_ENGINE_SUBSTITUTION,STRICT_TRANS_TABLES
EOF

proxy docker_on $HOST1 run -d -e MYSQL_ALLOW_EMPTY_PASSWORD=1 --name db-server -v /tmp/my.cnf:/etc/my.cnf percona/percona-server:5.6.28
proxy docker_on $HOST2 run --entrypoint=/opt/sysbench/sysbench percona/sysbench --test=/opt/tests/db/oltp.lua --oltp_tables_count=$TABLE_COUNT --oltp_table_size=$TABLE_SIZE --mysql-host=db-server --mysql-user=root prepare
proxy docker_on $HOST2 run --entrypoint=/opt/sysbench/sysbench percona/sysbench --test=/opt/tests/db/oltp.lua --oltp_tables_count=$TABLE_COUNT --oltp_table_size=$TABLE_SIZE --num-threads=$NUM_THREADS --mysql-host=db-server --mysql-user=root --oltp-read-only=on --max-time=5 --max-requests=0 --report-interval=10 run

end_suite
