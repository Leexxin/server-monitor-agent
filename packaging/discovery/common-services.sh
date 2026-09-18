#!/bin/sh
set -eu

# Output contract:
# category<TAB>product<TAB>display name<TAB>confidence<TAB>evidence

emit_for_service() {
    service_name=$1
    normalized=$(printf '%s' "$service_name" | tr '[:upper:]' '[:lower:]')
    case "$normalized" in
        nginx.service|nginx)
            printf 'middleware\tnginx\tNginx\thigh\tservice:%s\n' "$service_name"
            ;;
        apache2.service|httpd.service|apache2|httpd)
            printf 'middleware\tapache-httpd\tApache HTTP Server\thigh\tservice:%s\n' "$service_name"
            ;;
        tomcat*.service|tomcat*)
            printf 'middleware\ttomcat\tApache Tomcat\thigh\tservice:%s\n' "$service_name"
            ;;
        kafka*.service|kafka*)
            printf 'middleware\tkafka\tApache Kafka\thigh\tservice:%s\n' "$service_name"
            ;;
        zookeeper*.service|zookeeper*)
            printf 'middleware\tzookeeper\tApache ZooKeeper\thigh\tservice:%s\n' "$service_name"
            ;;
        rabbitmq-server.service|rabbitmq*)
            printf 'middleware\trabbitmq\tRabbitMQ\thigh\tservice:%s\n' "$service_name"
            ;;
        haproxy.service|haproxy)
            printf 'middleware\thaproxy\tHAProxy\thigh\tservice:%s\n' "$service_name"
            ;;
        mysql.service|mysqld.service|mysql|mysqld)
            printf 'database\tmysql\tMySQL\thigh\tservice:%s\n' "$service_name"
            ;;
        mariadb.service|mariadb)
            printf 'database\tmariadb\tMariaDB\thigh\tservice:%s\n' "$service_name"
            ;;
        postgresql*.service|postgresql*)
            printf 'database\tpostgresql\tPostgreSQL\thigh\tservice:%s\n' "$service_name"
            ;;
        mongod.service|mongodb.service|mongod|mongodb)
            printf 'database\tmongodb\tMongoDB\thigh\tservice:%s\n' "$service_name"
            ;;
        redis*.service|redis*)
            printf 'database\tredis\tRedis\thigh\tservice:%s\n' "$service_name"
            ;;
        elasticsearch.service|elasticsearch)
            printf 'database\telasticsearch\tElasticsearch\thigh\tservice:%s\n' "$service_name"
            ;;
        clickhouse-server.service|clickhouse-server)
            printf 'database\tclickhouse\tClickHouse\thigh\tservice:%s\n' "$service_name"
            ;;
        influxdb.service|influxdb)
            printf 'database\tinfluxdb\tInfluxDB\thigh\tservice:%s\n' "$service_name"
            ;;
        ssh.service|sshd.service|ssh|sshd)
            printf 'file_transfer\topenssh-sftp\tOpenSSH SFTP\tmedium\tservice:%s\n' "$service_name"
            ;;
        vsftpd.service|vsftpd)
            printf 'file_transfer\tvsftpd\tvsftpd\thigh\tservice:%s\n' "$service_name"
            ;;
        proftpd.service|proftpd)
            printf 'file_transfer\tproftpd\tProFTPD\thigh\tservice:%s\n' "$service_name"
            ;;
    esac
}

if command -v systemctl >/dev/null 2>&1; then
    systemctl list-units --type=service --state=running --no-legend --no-pager 2>/dev/null |
        while IFS=' ' read -r unit _rest; do
            [ -n "$unit" ] && emit_for_service "$unit"
        done
elif command -v rc-status >/dev/null 2>&1; then
    rc-status -s 2>/dev/null |
        while IFS=' ' read -r service_name _rest; do
            [ -n "$service_name" ] && emit_for_service "$service_name"
        done
fi
