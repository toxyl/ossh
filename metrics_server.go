package main

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type MetricsServer struct {
	addr             string
	logins           prometheus.Counter
	loginsFailed     prometheus.Counter
	loginsSuccessful prometheus.Counter
	commandsExecuted prometheus.Counter
	knownHosts       prometheus.Gauge
	knownPasswords   prometheus.Gauge
	knownUsers       prometheus.Gauge
	knownPayloads    prometheus.Gauge
	activeSessions   prometheus.Gauge
	timeOnline       prometheus.Gauge
	timeWasted       prometheus.Gauge
}

func (m *MetricsServer) IncrementLogins() {
	m.logins.Inc()
}

func (m *MetricsServer) IncrementFailedLogins() {
	m.loginsFailed.Inc()
}

func (m *MetricsServer) IncrementSuccessfulLogins() {
	m.loginsSuccessful.Inc()
}

func (m *MetricsServer) IncrementKnownHosts() {
	m.knownHosts.Inc()
}

func (m *MetricsServer) IncrementKnownPasswords() {
	m.knownPasswords.Inc()
}

func (m *MetricsServer) IncrementKnownUsers() {
	m.knownUsers.Inc()
}

func (m *MetricsServer) IncrementKnownPayloads() {
	m.knownPayloads.Inc()
}

func (m *MetricsServer) IncrementExecutedCommands() {
	m.commandsExecuted.Inc()
}

func (m *MetricsServer) IncrementSessions() {
	m.activeSessions.Inc()
}

func (m *MetricsServer) DecrementSessions() {
	m.activeSessions.Dec()
}

func (m *MetricsServer) SetTimeOnline(seconds float64) {
	m.timeOnline.Set(seconds)
}

func (m *MetricsServer) AddTimeWasted(seconds float64) {
	m.timeWasted.Add(seconds)
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
	}
	return m
}
