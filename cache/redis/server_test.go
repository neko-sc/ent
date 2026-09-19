// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package redis_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/redis/rueidis"
	"github.com/stretchr/testify/require"
)

var (
	redisAddress     string
	redisUnavailable string
)

func TestMain(tests *testing.M) {
	cleanup := startRedis()
	status := tests.Run()
	cleanup()
	os.Exit(status)
}

func startRedis() func() {
	if address := os.Getenv("REDIS_ADDR"); address != "" {
		if err := waitForRedis(address); err == nil {
			redisAddress = address
			fmt.Fprintln(os.Stderr, "redis integration: REDIS_ADDR", address)
			return func() {}
		} else {
			redisUnavailable = "REDIS_ADDR: " + err.Error()
		}
	}
	if binary, err := exec.LookPath("redis-server"); err == nil {
		address, port, err := freeAddress()
		if err == nil {
			command := exec.Command(binary, "--bind", "127.0.0.1", "--port", port, "--save", "", "--appendonly", "no")
			command.Stdout = io.Discard
			command.Stderr = io.Discard
			if err = command.Start(); err == nil {
				cleanup := func() {
					_ = command.Process.Kill()
					_ = command.Wait()
				}
				if err = waitForRedis(address); err == nil {
					redisAddress = address
					fmt.Fprintln(os.Stderr, "redis integration: redis-server", address)
					return cleanup
				}
				cleanup()
			}
		}
		redisUnavailable += fmt.Sprintf("; redis-server: %v", err)
	}
	if binary, err := exec.LookPath("docker"); err == nil {
		address, port, err := freeAddress()
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			output, startError := exec.CommandContext(ctx, binary, "run", "-d", "--rm", "-p", "127.0.0.1:"+port+":6379", "redis:7").Output()
			err = startError
			if err == nil {
				container := strings.TrimSpace(string(output))
				cleanup := func() {
					ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					defer cancel()
					if output, err := exec.CommandContext(ctx, binary, "stop", "--time", "1", container).CombinedOutput(); err != nil {
						fmt.Fprintf(os.Stderr, "redis integration cleanup: %v: %s\n", err, output)
					}
				}
				if err = waitForRedis(address); err == nil {
					redisAddress = address
					fmt.Fprintln(os.Stderr, "redis integration: docker redis:7", address)
					return cleanup
				}
				cleanup()
			}
		}
		redisUnavailable += fmt.Sprintf("; docker: %v", err)
	}
	redisUnavailable = "real RESP3 Redis unavailable (tried REDIS_ADDR, redis-server, docker)" + redisUnavailable
	fmt.Fprintln(os.Stderr, redisUnavailable)
	return func() {}
}

func freeAddress() (string, string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", "", err
	}
	address := listener.Addr().String()
	_, port, err := net.SplitHostPort(address)
	if closeError := listener.Close(); closeError != nil {
		return "", "", closeError
	}
	return address, port, err
}

func waitForRedis(address string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		client, err := rueidis.NewClient(rueidis.ClientOption{InitAddress: []string{address}, Dialer: net.Dialer{Timeout: 200 * time.Millisecond}, ConnWriteTimeout: time.Second})
		if err == nil {
			err = client.Do(ctx, client.B().Ping().Build()).Error()
			client.Close()
			if err == nil {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s: %w", address, err)
		case <-ticker.C:
		}
	}
}

func newClient(t *testing.T) rueidis.Client {
	t.Helper()
	if redisAddress == "" {
		t.Skip(redisUnavailable)
	}
	client, err := rueidis.NewClient(rueidis.ClientOption{InitAddress: []string{redisAddress}, AlwaysPipelining: true})
	require.NoError(t, err)
	t.Cleanup(client.Close)
	return client
}
