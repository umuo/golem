package main

import (
	"errors"
	"strings"
	"time"
)

type runtimeConfig struct {
	Listen             string
	Path               string
	AccessToken        string
	Trigger            string
	HeartbeatInterval  time.Duration
	WriteTimeout       time.Duration
	MaxFrameBytes      int64
	ClientBuffer       int
	RemoteMediaTimeout time.Duration
	MaxMediaBytes      int64
	AllowLocalFiles    bool
}

func normalizeConfig(config Config) (runtimeConfig, error) {
	defaults := defaultConfig()
	if strings.TrimSpace(config.Listen) == "" {
		config.Listen = defaults.Listen
	}
	config.Path = strings.TrimSpace(config.Path)
	if config.Path == "" {
		config.Path = defaults.Path
	}
	if !strings.HasPrefix(config.Path, "/") {
		config.Path = "/" + config.Path
	}
	if strings.ContainsAny(config.Path, "?#") {
		return runtimeConfig{}, errors.New("OneBot WebSocket path 不能包含 query 或 fragment")
	}
	config.Trigger = strings.TrimSpace(config.Trigger)
	if config.Trigger == "" {
		config.Trigger = defaults.Trigger
	}
	if config.WriteTimeoutSeconds <= 0 {
		config.WriteTimeoutSeconds = defaults.WriteTimeoutSeconds
	}
	if config.MaxFrameBytes <= 0 {
		config.MaxFrameBytes = defaults.MaxFrameBytes
	}
	if config.ClientBuffer <= 0 {
		config.ClientBuffer = defaults.ClientBuffer
	}
	if config.RemoteMediaTimeoutSeconds <= 0 {
		config.RemoteMediaTimeoutSeconds = defaults.RemoteMediaTimeoutSeconds
	}
	if config.MaxMediaBytes <= 0 {
		config.MaxMediaBytes = defaults.MaxMediaBytes
	}

	var heartbeat time.Duration
	if config.HeartbeatIntervalSeconds > 0 {
		heartbeat = time.Duration(config.HeartbeatIntervalSeconds) * time.Second
	}
	return runtimeConfig{
		Listen:             strings.TrimSpace(config.Listen),
		Path:               config.Path,
		AccessToken:        strings.TrimSpace(config.AccessToken),
		Trigger:            config.Trigger,
		HeartbeatInterval:  heartbeat,
		WriteTimeout:       time.Duration(config.WriteTimeoutSeconds) * time.Second,
		MaxFrameBytes:      config.MaxFrameBytes,
		ClientBuffer:       config.ClientBuffer,
		RemoteMediaTimeout: time.Duration(config.RemoteMediaTimeoutSeconds) * time.Second,
		MaxMediaBytes:      config.MaxMediaBytes,
		AllowLocalFiles:    config.AllowLocalFiles,
	}, nil
}
