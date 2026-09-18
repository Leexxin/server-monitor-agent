package discovery

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ProcessDetector struct {
	ProcPath      string
	SSHConfigPath string
}

type productDefinition struct {
	category    string
	product     string
	displayName string
	comm        []string
	command     []string
	ports       []int
}

var commonProducts = []productDefinition{
	{category: "middleware", product: "nginx", displayName: "Nginx", comm: []string{"nginx"}, ports: []int{80, 443}},
	{category: "middleware", product: "apache-httpd", displayName: "Apache HTTP Server", comm: []string{"apache2", "httpd"}, ports: []int{80, 443}},
	{category: "middleware", product: "tomcat", displayName: "Apache Tomcat", command: []string{"org.apache.catalina.startup.bootstrap", "catalina.base"}, ports: []int{8080, 8443}},
	{category: "middleware", product: "kafka", displayName: "Apache Kafka", command: []string{"kafka.kafka", "kafka.server"}, ports: []int{9092}},
	{category: "middleware", product: "zookeeper", displayName: "Apache ZooKeeper", command: []string{"quorumpeermain", "zookeeper"}, ports: []int{2181}},
	{category: "middleware", product: "rabbitmq", displayName: "RabbitMQ", comm: []string{"beam.smp", "rabbitmq-server"}, command: []string{"rabbitmq"}, ports: []int{5672, 15672}},
	{category: "middleware", product: "haproxy", displayName: "HAProxy", comm: []string{"haproxy"}, ports: []int{80, 443}},
	{category: "middleware", product: "envoy", displayName: "Envoy Proxy", comm: []string{"envoy"}, ports: []int{10000}},
	{category: "database", product: "mysql", displayName: "MySQL", comm: []string{"mysqld", "mysql-server"}, ports: []int{3306}},
	{category: "database", product: "mariadb", displayName: "MariaDB", comm: []string{"mariadbd"}, ports: []int{3306}},
	{category: "database", product: "postgresql", displayName: "PostgreSQL", comm: []string{"postgres", "postmaster"}, ports: []int{5432}},
	{category: "database", product: "mongodb", displayName: "MongoDB", comm: []string{"mongod"}, ports: []int{27017}},
	{category: "database", product: "redis", displayName: "Redis", comm: []string{"redis-server"}, ports: []int{6379}},
	{category: "database", product: "elasticsearch", displayName: "Elasticsearch", command: []string{"org.elasticsearch.bootstrap.elasticsearch"}, ports: []int{9200, 9300}},
	{category: "database", product: "clickhouse", displayName: "ClickHouse", comm: []string{"clickhouse-server"}, ports: []int{8123, 9000}},
	{category: "database", product: "influxdb", displayName: "InfluxDB", comm: []string{"influxd"}, ports: []int{8086}},
	{category: "file_transfer", product: "openssh-sftp", displayName: "OpenSSH SFTP", comm: []string{"sshd"}, ports: []int{22}},
	{category: "file_transfer", product: "vsftpd", displayName: "vsftpd", comm: []string{"vsftpd"}, ports: []int{21}},
	{category: "file_transfer", product: "proftpd", displayName: "ProFTPD", comm: []string{"proftpd"}, ports: []int{21}},
}

func (ProcessDetector) Name() string { return "process" }

func (d ProcessDetector) Detect(ctx context.Context) ([]Asset, error) {
	entries, err := os.ReadDir(d.ProcPath)
	if err != nil {
		return nil, fmt.Errorf("read procfs: %w", err)
	}
	listening := readListeningPorts(d.ProcPath)
	found := make(map[string]*Asset)
	now := time.Now().UTC()
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || !entry.IsDir() {
			continue
		}
		commData, err := readLimitedFile(filepath.Join(d.ProcPath, entry.Name(), "comm"), 256)
		if err != nil {
			continue
		}
		comm := strings.ToLower(strings.TrimSpace(string(commData)))
		cmdData, _ := readLimitedFile(filepath.Join(d.ProcPath, entry.Name(), "cmdline"), 64<<10)
		command := strings.ToLower(string(bytes.ReplaceAll(cmdData, []byte{0}, []byte{' '})))
		for _, definition := range commonProducts {
			if !matchesProduct(definition, comm, command) {
				continue
			}
			asset := found[definition.category+"/"+definition.product]
			if asset == nil {
				asset = &Asset{
					Category: definition.category, Product: definition.product, DisplayName: definition.displayName,
					Status: "running", Confidence: "high", Ports: []Port{}, Evidence: []Evidence{}, DetectedAt: now,
				}
				for _, port := range definition.ports {
					if listening[port] {
						asset.Ports = append(asset.Ports, Port{Protocol: "tcp", Port: port})
					}
				}
				found[definition.category+"/"+definition.product] = asset
			}
			asset.Evidence = append(asset.Evidence, Evidence{Source: "process", Value: comm, PID: pid})
		}
	}
	d.detectSFTPConfig(found, now)

	assets := make([]Asset, 0, len(found))
	for _, asset := range found {
		assets = append(assets, *asset)
	}
	sort.Slice(assets, func(i, j int) bool {
		if assets[i].Category == assets[j].Category {
			return assets[i].Product < assets[j].Product
		}
		return assets[i].Category < assets[j].Category
	})
	return assets, nil
}

func (d ProcessDetector) detectSFTPConfig(found map[string]*Asset, now time.Time) {
	path := d.SSHConfigPath
	if path == "" {
		path = "/etc/ssh/sshd_config"
	}
	data, err := readLimitedFile(path, 1<<20)
	if err != nil {
		return
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(strings.ToLower(line))
		if len(fields) < 3 || fields[0] != "subsystem" || fields[1] != "sftp" {
			continue
		}
		key := "file_transfer/openssh-sftp"
		asset := found[key]
		if asset == nil {
			asset = &Asset{
				Category: "file_transfer", Product: "openssh-sftp", DisplayName: "OpenSSH SFTP",
				Status: "configured", Confidence: "medium", Ports: []Port{}, Evidence: []Evidence{}, DetectedAt: now,
			}
			found[key] = asset
		}
		asset.Evidence = append(asset.Evidence, Evidence{Source: "configuration", Value: "sshd Subsystem sftp"})
		if asset.Status == "running" {
			asset.Confidence = "high"
		}
		break
	}
}

func matchesProduct(definition productDefinition, comm, command string) bool {
	for _, candidate := range definition.comm {
		if comm == candidate {
			if definition.product == "rabbitmq" && comm == "beam.smp" && !strings.Contains(command, "rabbit") {
				continue
			}
			return true
		}
	}
	for _, candidate := range definition.command {
		if strings.Contains(command, candidate) {
			return true
		}
	}
	return false
}

func readListeningPorts(procPath string) map[int]bool {
	ports := make(map[int]bool)
	for _, name := range []string{"tcp", "tcp6"} {
		f, err := os.Open(filepath.Join(procPath, "net", name))
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) < 4 || fields[3] != "0A" {
				continue
			}
			separator := strings.LastIndexByte(fields[1], ':')
			if separator < 0 {
				continue
			}
			value, err := strconv.ParseUint(fields[1][separator+1:], 16, 16)
			if err == nil {
				ports[int(value)] = true
			}
		}
		f.Close()
	}
	return ports
}

func readLimitedFile(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, limit))
}
