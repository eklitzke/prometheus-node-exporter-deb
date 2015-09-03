// +build !nonetstat

package collector

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	procNetStat       = "/proc/net/netstat"
	procSNMPStat      = "/proc/net/snmp"
	netStatsSubsystem = "netstat"
)

type netStatCollector struct {
	metrics map[string]prometheus.Gauge
}

func init() {
	Factories["netstat"] = NewNetStatCollector
}

// NewNetStatCollector takes a returns
// a new Collector exposing network stats.
func NewNetStatCollector() (Collector, error) {
	return &netStatCollector{
		metrics: map[string]prometheus.Gauge{},
	}, nil
}

func (c *netStatCollector) Update(ch chan<- prometheus.Metric) (err error) {
	netStats, err := getNetStats(procNetStat)
	if err != nil {
		return fmt.Errorf("couldn't get netstats: %s", err)
	}
	snmpStats, err := getNetStats(procSNMPStat)
	if err != nil {
		return fmt.Errorf("couldn't get SNMP stats: %s", err)
	}
	// Merge the results of snmpStats into netStats (collisions are possible, but
	// we know that the keys are always unique for the given use case)
	for k, v := range snmpStats {
		netStats[k] = v
	}
	for protocol, protocolStats := range netStats {
		for name, value := range protocolStats {
			key := protocol + "_" + name
			if _, ok := c.metrics[key]; !ok {
				c.metrics[key] = prometheus.NewGauge(
					prometheus.GaugeOpts{
						Namespace: Namespace,
						Subsystem: netStatsSubsystem,
						Name:      key,
						Help:      fmt.Sprintf("%s %s from /proc/net/{netstat,snmp}.", protocol, name),
					},
				)
			}
			v, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return fmt.Errorf("invalid value %s in netstats: %s", value, err)
			}
			c.metrics[key].Set(v)
		}
	}
	for _, m := range c.metrics {
		m.Collect(ch)
	}
	return err
}

func getNetStats(fileName string) (map[string]map[string]string, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return parseNetStats(file, fileName)
}

func parseNetStats(r io.Reader, fileName string) (map[string]map[string]string, error) {
	var (
		netStats = map[string]map[string]string{}
		scanner  = bufio.NewScanner(r)
	)

	for scanner.Scan() {
		nameParts := strings.Split(string(scanner.Text()), " ")
		scanner.Scan()
		valueParts := strings.Split(string(scanner.Text()), " ")
		// Remove trailing :.
		protocol := nameParts[0][:len(nameParts[0])-1]
		netStats[protocol] = map[string]string{}
		if len(nameParts) != len(valueParts) {
			return nil, fmt.Errorf("mismatch field count mismatch in %s: %s",
				fileName, protocol)
		}
		for i := 1; i < len(nameParts); i++ {
			netStats[protocol][nameParts[i]] = valueParts[i]
		}
	}

	return netStats, nil
}
