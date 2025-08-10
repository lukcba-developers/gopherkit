package events

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

// NATSClient provides event streaming capabilities using NATS
type NATSClient struct {
	conn    *nats.Conn
	js      nats.JetStreamContext
	logger  *logrus.Logger
	config  NATSConfig
	mu      sync.RWMutex
	streams map[string]*StreamInfo
}

// NATSConfig holds NATS client configuration
type NATSConfig struct {
	URL              string        `json:"url"`
	ClusterID        string        `json:"cluster_id"`
	ClientID         string        `json:"client_id"`
	MaxReconnect     int           `json:"max_reconnect"`
	ReconnectWait    time.Duration `json:"reconnect_wait"`
	Timeout          time.Duration `json:"timeout"`
	EnableJetStream  bool          `json:"enable_jetstream"`
	StreamConfig     StreamConfig  `json:"stream_config"`
}

// StreamConfig holds JetStream configuration
type StreamConfig struct {
	Name         string        `json:"name"`
	Subjects     []string      `json:"subjects"`
	Retention    string        `json:"retention"`    // WorkQueue, Interest, Limits
	MaxAge       time.Duration `json:"max_age"`
	MaxBytes     int64         `json:"max_bytes"`
	MaxMsgs      int64         `json:"max_msgs"`
	Replicas     int           `json:"replicas"`
	Storage      string        `json:"storage"`      // File, Memory
}

// StreamInfo contains information about a stream
type StreamInfo struct {
	Name      string   `json:"name"`
	Subjects  []string `json:"subjects"`
	CreatedAt time.Time `json:"created_at"`
	Messages  uint64   `json:"messages"`
	Bytes     uint64   `json:"bytes"`
	State     string   `json:"state"`
}

// EventMessage represents an event message
type EventMessage struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Source      string                 `json:"source"`
	Subject     string                 `json:"subject"`
	Data        map[string]interface{} `json:"data"`
	Timestamp   time.Time              `json:"timestamp"`
	Version     string                 `json:"version"`
	CorrelationID string               `json:"correlation_id,omitempty"`
	Headers     map[string]string      `json:"headers,omitempty"`
}

// EventHandler defines the interface for event handlers
type EventHandler interface {
	Handle(ctx context.Context, event *EventMessage) error
	GetSubject() string
	GetName() string
}

// EventSubscription represents an active subscription
type EventSubscription struct {
	Subject     string           `json:"subject"`
	Handler     EventHandler     `json:"-"`
	Subscription *nats.Subscription `json:"-"`
	CreatedAt   time.Time        `json:"created_at"`
	IsActive    bool             `json:"is_active"`
	MessageCount uint64          `json:"message_count"`
}

// NewNATSClient creates a new NATS client
func NewNATSClient(config NATSConfig, logger *logrus.Logger) (*NATSClient, error) {
	// Set default values
	if config.MaxReconnect == 0 {
		config.MaxReconnect = -1 // Unlimited
	}
	if config.ReconnectWait == 0 {
		config.ReconnectWait = 2 * time.Second
	}
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}

	// Configure NATS connection options
	opts := []nats.Option{
		nats.Name(config.ClientID),
		nats.MaxReconnects(config.MaxReconnect),
		nats.ReconnectWait(config.ReconnectWait),
		nats.Timeout(config.Timeout),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			logger.WithError(err).Warn("NATS disconnected")
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			logger.WithField("url", nc.ConnectedUrl()).Info("NATS reconnected")
		}),
		nats.ClosedHandler(func(nc *nats.Conn) {
			logger.Info("NATS connection closed")
		}),
		nats.ErrorHandler(func(nc *nats.Conn, sub *nats.Subscription, err error) {
			logger.WithFields(logrus.Fields{
				"subject": sub.Subject,
				"error":   err,
			}).Error("NATS subscription error")
		}),
	}

	// Connect to NATS
	conn, err := nats.Connect(config.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	client := &NATSClient{
		conn:    conn,
		logger:  logger,
		config:  config,
		streams: make(map[string]*StreamInfo),
	}

	// Initialize JetStream if enabled
	if config.EnableJetStream {
		js, err := conn.JetStream()
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("failed to initialize JetStream: %w", err)
		}
		client.js = js

		// Create default stream if configured
		if config.StreamConfig.Name != "" {
			if err := client.createStream(config.StreamConfig); err != nil {
				logger.WithError(err).Warn("Failed to create default stream, continuing without it")
			}
		}
	}

	logger.WithFields(logrus.Fields{
		"url":        config.URL,
		"client_id":  config.ClientID,
		"jetstream":  config.EnableJetStream,
	}).Info("NATS client initialized successfully")

	return client, nil
}

// PublishEvent publishes an event to a subject
func (nc *NATSClient) PublishEvent(ctx context.Context, event *EventMessage) error {
	// Validate event
	if err := nc.validateEvent(event); err != nil {
		return fmt.Errorf("invalid event: %w", err)
	}

	// Set timestamp if not provided
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Serialize event
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to serialize event: %w", err)
	}

	// Create NATS message
	msg := &nats.Msg{
		Subject: event.Subject,
		Data:    data,
	}

	// Add headers if provided
	if len(event.Headers) > 0 {
		msg.Header = make(nats.Header)
		for k, v := range event.Headers {
			msg.Header.Set(k, v)
		}
	}

	// Add correlation ID header
	if event.CorrelationID != "" {
		if msg.Header == nil {
			msg.Header = make(nats.Header)
		}
		msg.Header.Set("Correlation-ID", event.CorrelationID)
	}

	// Publish with JetStream if available, otherwise use core NATS
	if nc.js != nil {
		_, err = nc.js.PublishMsg(msg)
	} else {
		err = nc.conn.PublishMsg(msg)
	}

	if err != nil {
		nc.logger.WithFields(logrus.Fields{
			"subject":        event.Subject,
			"type":           event.Type,
			"correlation_id": event.CorrelationID,
			"error":          err,
		}).Error("Failed to publish event")
		return fmt.Errorf("failed to publish event: %w", err)
	}

	nc.logger.WithFields(logrus.Fields{
		"subject":        event.Subject,
		"type":           event.Type,
		"correlation_id": event.CorrelationID,
	}).Debug("Event published successfully")

	return nil
}

// Subscribe creates a subscription to a subject with a handler
func (nc *NATSClient) Subscribe(handler EventHandler) (*EventSubscription, error) {
	subject := handler.GetSubject()
	
	// Create message handler wrapper
	msgHandler := func(msg *nats.Msg) {
		ctx := context.Background()
		
		// Parse event from message
		var event EventMessage
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			nc.logger.WithFields(logrus.Fields{
				"subject": subject,
				"error":   err,
			}).Error("Failed to parse event message")
			return
		}

		// Extract correlation ID from headers
		if msg.Header != nil {
			if corrID := msg.Header.Get("Correlation-ID"); corrID != "" {
				event.CorrelationID = corrID
			}
		}

		// Handle the event
		if err := handler.Handle(ctx, &event); err != nil {
			nc.logger.WithFields(logrus.Fields{
				"subject":        subject,
				"handler":        handler.GetName(),
				"correlation_id": event.CorrelationID,
				"error":          err,
			}).Error("Event handler failed")
			return
		}

		nc.logger.WithFields(logrus.Fields{
			"subject":        subject,
			"handler":        handler.GetName(),
			"correlation_id": event.CorrelationID,
		}).Debug("Event handled successfully")
	}

	// Create subscription
	var sub *nats.Subscription
	var err error

	if nc.js != nil {
		// Use JetStream subscription for durability
		sub, err = nc.js.Subscribe(subject, msgHandler, nats.Durable(handler.GetName()))
	} else {
		// Use core NATS subscription
		sub, err = nc.conn.Subscribe(subject, msgHandler)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create subscription: %w", err)
	}

	subscription := &EventSubscription{
		Subject:      subject,
		Handler:      handler,
		Subscription: sub,
		CreatedAt:    time.Now(),
		IsActive:     true,
	}

	nc.logger.WithFields(logrus.Fields{
		"subject": subject,
		"handler": handler.GetName(),
	}).Info("Subscription created successfully")

	return subscription, nil
}

// QueueSubscribe creates a queue subscription for load balancing
func (nc *NATSClient) QueueSubscribe(handler EventHandler, queueGroup string) (*EventSubscription, error) {
	subject := handler.GetSubject()
	
	// Create message handler wrapper
	msgHandler := func(msg *nats.Msg) {
		ctx := context.Background()
		
		var event EventMessage
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			nc.logger.WithFields(logrus.Fields{
				"subject": subject,
				"queue":   queueGroup,
				"error":   err,
			}).Error("Failed to parse event message")
			return
		}

		// Extract correlation ID from headers
		if msg.Header != nil {
			if corrID := msg.Header.Get("Correlation-ID"); corrID != "" {
				event.CorrelationID = corrID
			}
		}

		if err := handler.Handle(ctx, &event); err != nil {
			nc.logger.WithFields(logrus.Fields{
				"subject":        subject,
				"queue":          queueGroup,
				"handler":        handler.GetName(),
				"correlation_id": event.CorrelationID,
				"error":          err,
			}).Error("Event handler failed")
			return
		}

		nc.logger.WithFields(logrus.Fields{
			"subject":        subject,
			"queue":          queueGroup,
			"handler":        handler.GetName(),
			"correlation_id": event.CorrelationID,
		}).Debug("Event handled successfully")
	}

	// Create queue subscription
	var sub *nats.Subscription
	var err error

	if nc.js != nil {
		// Use JetStream queue subscription
		sub, err = nc.js.QueueSubscribe(subject, queueGroup, msgHandler, nats.Durable(handler.GetName()))
	} else {
		// Use core NATS queue subscription
		sub, err = nc.conn.QueueSubscribe(subject, queueGroup, msgHandler)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create queue subscription: %w", err)
	}

	subscription := &EventSubscription{
		Subject:      subject,
		Handler:      handler,
		Subscription: sub,
		CreatedAt:    time.Now(),
		IsActive:     true,
	}

	nc.logger.WithFields(logrus.Fields{
		"subject": subject,
		"queue":   queueGroup,
		"handler": handler.GetName(),
	}).Info("Queue subscription created successfully")

	return subscription, nil
}

// Unsubscribe removes a subscription
func (nc *NATSClient) Unsubscribe(subscription *EventSubscription) error {
	if !subscription.IsActive {
		return fmt.Errorf("subscription is not active")
	}

	if err := subscription.Subscription.Unsubscribe(); err != nil {
		return fmt.Errorf("failed to unsubscribe: %w", err)
	}

	subscription.IsActive = false

	nc.logger.WithFields(logrus.Fields{
		"subject": subscription.Subject,
		"handler": subscription.Handler.GetName(),
	}).Info("Subscription removed successfully")

	return nil
}

// CreateStream creates a JetStream stream
func (nc *NATSClient) CreateStream(config StreamConfig) error {
	if nc.js == nil {
		return fmt.Errorf("JetStream not enabled")
	}

	return nc.createStream(config)
}

// GetStreamInfo returns information about a stream
func (nc *NATSClient) GetStreamInfo(streamName string) (*StreamInfo, error) {
	if nc.js == nil {
		return nil, fmt.Errorf("JetStream not enabled")
	}

	info, err := nc.js.StreamInfo(streamName)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream info: %w", err)
	}

	return &StreamInfo{
		Name:      info.Config.Name,
		Subjects:  info.Config.Subjects,
		CreatedAt: info.Created,
		Messages:  info.State.Msgs,
		Bytes:     info.State.Bytes,
		State:     "active",
	}, nil
}

// ListStreams returns a list of all streams
func (nc *NATSClient) ListStreams() ([]*StreamInfo, error) {
	if nc.js == nil {
		return nil, fmt.Errorf("JetStream not enabled")
	}

	names := nc.js.StreamNames()
	var streams []*StreamInfo

	for name := range names {
		info, err := nc.GetStreamInfo(name)
		if err != nil {
			nc.logger.WithFields(logrus.Fields{
				"stream": name,
				"error":  err,
			}).Warn("Failed to get stream info")
			continue
		}
		streams = append(streams, info)
	}

	return streams, nil
}

// GetStats returns connection statistics
func (nc *NATSClient) GetStats() nats.Statistics {
	return nc.conn.Stats()
}

// IsConnected returns the connection status
func (nc *NATSClient) IsConnected() bool {
	return nc.conn.IsConnected()
}

// HealthCheck performs a health check
func (nc *NATSClient) HealthCheck(ctx context.Context) error {
	if !nc.conn.IsConnected() {
		return fmt.Errorf("NATS connection is not active")
	}

	// Test basic connectivity with a ping
	if err := nc.conn.FlushTimeout(time.Second); err != nil {
		return fmt.Errorf("NATS flush failed: %w", err)
	}

	return nil
}

// Close closes the NATS connection
func (nc *NATSClient) Close() error {
	if nc.conn != nil {
		nc.conn.Close()
		nc.logger.Info("NATS connection closed")
	}
	return nil
}

// Private helper methods

func (nc *NATSClient) createStream(config StreamConfig) error {
	// Set default storage type
	if config.Storage == "" {
		config.Storage = "file"
	}

	// Set default retention policy
	if config.Retention == "" {
		config.Retention = "limits"
	}

	// Convert retention string to NATS constant
	var retention nats.RetentionPolicy
	switch config.Retention {
	case "limits":
		retention = nats.LimitsPolicy
	case "interest":
		retention = nats.InterestPolicy
	case "workqueue":
		retention = nats.WorkQueuePolicy
	default:
		retention = nats.LimitsPolicy
	}

	// Convert storage string to NATS constant
	var storage nats.StorageType
	switch config.Storage {
	case "file":
		storage = nats.FileStorage
	case "memory":
		storage = nats.MemoryStorage
	default:
		storage = nats.FileStorage
	}

	streamConfig := &nats.StreamConfig{
		Name:      config.Name,
		Subjects:  config.Subjects,
		Retention: retention,
		MaxAge:    config.MaxAge,
		MaxBytes:  config.MaxBytes,
		MaxMsgs:   config.MaxMsgs,
		Replicas:  config.Replicas,
		Storage:   storage,
	}

	_, err := nc.js.AddStream(streamConfig)
	if err != nil {
		return fmt.Errorf("failed to create stream: %w", err)
	}

	// Update internal streams map
	nc.mu.Lock()
	nc.streams[config.Name] = &StreamInfo{
		Name:      config.Name,
		Subjects:  config.Subjects,
		CreatedAt: time.Now(),
		State:     "active",
	}
	nc.mu.Unlock()

	nc.logger.WithFields(logrus.Fields{
		"stream":   config.Name,
		"subjects": config.Subjects,
		"storage":  config.Storage,
	}).Info("Stream created successfully")

	return nil
}

func (nc *NATSClient) validateEvent(event *EventMessage) error {
	if event.Subject == "" {
		return fmt.Errorf("subject is required")
	}

	if event.Type == "" {
		return fmt.Errorf("type is required")
	}

	if event.Source == "" {
		return fmt.Errorf("source is required")
	}

	if event.Data == nil {
		return fmt.Errorf("data is required")
	}

	return nil
}