package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dddance_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status_code"},
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "dddance_http_request_duration_seconds",
			Help:    "HTTP request latency",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	DBQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "dddance_db_query_duration_seconds",
			Help:    "Database query latency",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"query_name"},
	)

	KafkaMessagesPublished = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dddance_kafka_messages_published_total",
			Help: "Total Kafka messages published",
		},
		[]string{"topic"},
	)

	KafkaMessagesConsumed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dddance_kafka_messages_consumed_total",
			Help: "Total Kafka messages consumed",
		},
		[]string{"topic", "group"},
	)

	ActiveDuels = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "dddance_active_duels",
			Help: "Number of currently active duels",
		},
	)

	AchievementsUnlocked = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dddance_achievements_unlocked_total",
			Help: "Total achievements unlocked by code",
		},
		[]string{"code"},
	)
)

func ObserveDBQuery(queryName string, fn func() error) error {
	start := time.Now()
	err := fn()
	DBQueryDuration.WithLabelValues(queryName).Observe(time.Since(start).Seconds())
	return err
}
