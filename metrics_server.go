package main

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type MetricsServer struct {
	addr                      string
	logins                    prometheus.Counter
	loginsPerSecond           prometheus.Gauge
	loginsFailed              prometheus.Counter
	loginsSuccessful          prometheus.Counter
	commandsExecuted          prometheus.Counter
	commandsExecutedPerSecond prometheus.Gauge
	knownHosts                prometheus.Gauge
	knownPasswords            prometheus.Gauge
	knownUsers                prometheus.Gauge
	knownPayloads             prometheus.Gauge
	activeSessions            prometheus.Gauge
	timeOnline                prometheus.Gauge
	timeWasted                prometheus.Gauge
	timeWastedPerSecond       prometheus.Gauge
	last                      struct {
		logins           int
		loginsFailed     int
		loginsSuccessful int
		commandsExecuted int
		knownHosts       int
		knownUsers       int
		knownPasswords   int
		knownPayloads    int
		activeSessions   int
		timeOnline       float64
		timeWasted       float64
	}
}

func (m *MetricsServer) IncrementLogins() {
	m.logins.Inc()
	m.last.logins++
	m.loginsPerSecond.Set(float64(m.last.logins) / m.last.timeOnline)
}

func (m *MetricsServer) IncrementFailedLogins() {
	m.loginsFailed.Inc()
	m.last.loginsFailed++
}

func (m *MetricsServer) IncrementSuccessfulLogins() {
	m.loginsSuccessful.Inc()
	m.last.loginsSuccessful++
}

func (m *MetricsServer) IncrementKnownHosts() {
	m.knownHosts.Inc()
	m.last.knownHosts++
}

func (m *MetricsServer) IncrementKnownPasswords() {
	m.knownPasswords.Inc()
	m.last.knownPasswords++
}

func (m *MetricsServer) IncrementKnownUsers() {
	m.knownUsers.Inc()
	m.last.knownUsers++
}

func (m *MetricsServer) IncrementKnownPayloads() {
	m.knownPayloads.Inc()
	m.last.knownPayloads++
}

func (m *MetricsServer) IncrementExecutedCommands() {
	m.commandsExecuted.Inc()
	m.last.commandsExecuted++
	m.commandsExecutedPerSecond.Set(float64(m.last.commandsExecuted) / m.last.timeOnline)
}

func (m *MetricsServer) IncrementSessions() {
	m.activeSessions.Inc()
	m.last.activeSessions++
}

func (m *MetricsServer) DecrementSessions() {
	m.activeSessions.Dec()
	m.last.activeSessions--
}

func (m *MetricsServer) SetTimeOnline(seconds float64) {
	m.timeOnline.Set(seconds)
	m.last.timeOnline = seconds
	m.timeWastedPerSecond.Set(m.last.timeWasted / m.last.timeOnline)
}

func (m *MetricsServer) AddTimeWasted(seconds float64) {
	m.timeWasted.Add(seconds)
	m.last.timeWasted += seconds
}

func (m *MetricsServer) Start() {
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		_ = http.ListenAndServe(m.addr, nil)
	}()
}

func NewMetricsServer() *MetricsServer {
	m := &MetricsServer{
		addr: fmt.Sprintf("%s:%d", Conf.MetricsServer.Host, Conf.MetricsServer.Port),
		logins: promauto.NewCounter(prometheus.CounterOpts{
			Name: "ossh_logins",
			Help: "The total number of logins",
		}),
		loginsPerSecond: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "ossh_logins_per_second",
			Help: "The number of logins to this instance per second of online time",
		}),
		loginsFailed: promauto.NewCounter(prometheus.CounterOpts{
			Name: "ossh_logins_failed",
			Help: "The total number of failed logins",
		}),
		loginsSuccessful: promauto.NewCounter(prometheus.CounterOpts{
			Name: "ossh_logins_successful",
			Help: "The total number of successful logins",
		}),
		commandsExecuted: promauto.NewCounter(prometheus.CounterOpts{
			Name: "ossh_commands_executed",
			Help: "The total number of commands executed in SSH sessions",
		}),
		commandsExecutedPerSecond: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "ossh_commands_executed_per_second",
			Help: "The number of commands this instance has executed per second of online time",
		}),
		knownHosts: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "ossh_known_hosts",
			Help: "The number of hosts known to this instance",
		}),
		knownPasswords: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "ossh_known_passwords",
			Help: "The number of passwords known to this instance",
		}),
		knownUsers: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "ossh_known_users",
			Help: "The number of user names known to this instance",
		}),
		knownPayloads: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "ossh_known_payloads",
			Help: "The number of payloads known to this instance",
		}),
		activeSessions: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "ossh_active_sessions",
			Help: "The number of SSH session currently open on this instance",
		}),
		timeOnline: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "ossh_time_online",
			Help: "The number of seconds this instance has been running",
		}),
		timeWasted: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "ossh_time_wasted",
			Help: "The number of bot seconds this instance has wasted",
		}),
		timeWastedPerSecond: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "ossh_time_wasted_per_second",
			Help: "The number of bot seconds this instance has wasted per second of online time",
		}),
		last: struct {
			logins           int
			loginsFailed     int
			loginsSuccessful int
			commandsExecuted int
			knownHosts       int
			knownUsers       int
			knownPasswords   int
			knownPayloads    int
			activeSessions   int
			timeOnline       float64
			timeWasted       float64
		}{
			logins:           0,
			loginsFailed:     0,
			loginsSuccessful: 0,
			commandsExecuted: 0,
			knownHosts:       0,
			knownUsers:       0,
			knownPasswords:   0,
			knownPayloads:    0,
			activeSessions:   0,
			timeOnline:       0.0,
			timeWasted:       0.0,
		},
	}
	return m
}
