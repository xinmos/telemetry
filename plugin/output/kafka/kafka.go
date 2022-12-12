package kafka

import (
	"encoding/json"
	"fmt"

	"github.com/Shopify/sarama"
	"github.com/gofrs/uuid"
	"github.com/sirupsen/logrus"

	"telemetry/models"
	"telemetry/plugin/common/kafka"
	"telemetry/plugin/common/proxy"
	"telemetry/plugin/serializers"
)

type Kafka struct {
	Brokers    []string `json:"brokers"`
	Topic      string   `json:"topic"`
	RoutingKey string   `json:"routing_key"`

	proxy.Socks5ProxyConfig

	// Legacy TLS config options
	// TLS client certificate
	Certificate string
	// TLS client key
	Key string
	// TLS certificate authority
	CA string

	kafka.WriteConfig

	log *logrus.Entry

	saramaConfig *sarama.Config
	producer     sarama.SyncProducer

	serializer serializers.Serializer
}

func (k *Kafka) SetSerializer(serializer serializers.Serializer) {
	k.serializer = serializer
}

func NewKafka() *Kafka {
	return &Kafka{
		log: models.NewLogger("outputs.kafka"),
	}
}

func (k *Kafka) Init() error {
	sarama.Logger = models.NewLogger("outputs.kafka.sarama")

	config := sarama.NewConfig()
	if err := k.SetConfig(config, k.log); err != nil {
		return err
	}
	k.saramaConfig = config

	// Legacy support ssl config
	if k.Certificate != "" {
		k.TLSCert = k.Certificate
		k.TLSCA = k.CA
		k.TLSKey = k.Key
	}

	if k.Socks5ProxyEnabled {
		config.Net.Proxy.Enable = true

		dialer, err := k.Socks5ProxyConfig.GetDialer()
		if err != nil {
			return fmt.Errorf("connecting to proxy server failed: %s", err)
		}
		config.Net.Proxy.Dialer = dialer
	}

	return nil
}

func (k *Kafka) Connect() error {
	producer, err := sarama.NewSyncProducer(k.Brokers, k.saramaConfig)
	if err != nil {
		return err
	}
	k.producer = producer
	return nil
}

func (k *Kafka) Close() error {
	return k.producer.Close()
}

func (k *Kafka) routingKey() (string, error) {
	if k.RoutingKey == "random" {
		u, err := uuid.NewV4()
		if err != nil {
			return "", err
		}
		return u.String(), nil
	}
	return k.RoutingKey, nil
}

func (k *Kafka) Write(metrics []models.Metric) error {
	msgs := make([]*sarama.ProducerMessage, 0, len(metrics))
	for _, metric := range metrics {
		buf, err := k.serializer.Serialize(metric)
		if err != nil {
			k.log.Debugf("Could not serialize metric: %v", err)
		}

		m := &sarama.ProducerMessage{
			Topic: k.Topic,
			Value: sarama.ByteEncoder(buf),
		}

		key, err := k.routingKey()
		if err != nil {
			return fmt.Errorf("could not generate routing key: %v", err)
		}

		if key != "" {
			m.Key = sarama.StringEncoder(key)
		}
		msgs = append(msgs, m)
	}

	err := k.producer.SendMessages(msgs)
	if err != nil {
		// We could have many errors, return only the first encountered.
		if errs, ok := err.(sarama.ProducerErrors); ok {
			for _, prodErr := range errs {
				if prodErr.Err == sarama.ErrMessageSizeTooLarge {
					k.log.Error("Message too large, consider increasing `max_message_bytes`; dropping batch")
					return nil
				}
				if prodErr.Err == sarama.ErrInvalidTimestamp {
					k.log.Error(
						"The timestamp of the message is out of acceptable range, consider increasing broker `message.timestamp.difference.max.ms`; " +
							"dropping batch",
					)
					return nil
				}
				return prodErr //nolint:staticcheck // Return first error encountered
			}
		}
		return err
	}

	return nil
}

func (k *Kafka) ParseConfig(cfg map[string]any) error {
	tmp, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	err = json.Unmarshal(tmp, k)
	if err != nil {
		return fmt.Errorf("[kafka] config error: %v", err)
	}
	return nil
}
