// Package enrollment provides agent self-enrollment with the Connection Manager.
package enrollment

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/edr-platform/win-agent/internal/config"
	grpcclient "github.com/edr-platform/win-agent/internal/grpc"
	"github.com/edr-platform/win-agent/internal/logging"
	pb "github.com/edr-platform/win-agent/internal/pb"
)

const registerTimeout = 30 * time.Second

// EnsureEnrolled ensures the agent has a valid client certificate. If cert and key
// already exist at the configured paths, it returns nil. Otherwise it performs
// registration with the Connection Manager using the bootstrap token and CSR,
// then saves the issued certificate and updates the config with the assigned agent ID.
// If configFilePath is non-empty and a new AgentID was received, the config is persisted to that file.
func EnsureEnrolled(cfg *config.Config, logger *logging.Logger, configFilePath string) error {
	if cfg == nil {
		return errors.New("config is required")
	}
	if logger == nil {
		return errors.New("logger is required")
	}

	// Already enrolled if both cert and key exist
	if _, err := os.Stat(cfg.Certs.CertPath); err == nil {
		if _, err := os.Stat(cfg.Certs.KeyPath); err == nil {
			logger.Info("Agent already enrolled")
			return nil
		}
	}

	cm := grpcclient.NewCertManagerFromConfig(cfg, logger)

	if cfg.Certs.BootstrapToken == "" {
		return errors.New("bootstrap token is required for enrollment; set certs.bootstrap_token in config")
	}

	csrPEM, err := cm.GenerateCSR(cfg.Agent.ID, cfg.Agent.Hostname)
	if err != nil {
		return fmt.Errorf("generate CSR: %w", err)
	}

	// Dial the Connection Manager for enrollment.
	// Use TLS with CA cert only (no client cert — we're trying to obtain one).
	// The server's tls.Config uses VerifyClientCertIfGiven, so this works.
	var dialOpt grpc.DialOption
	if cfg.Server.Insecure {
		dialOpt = grpc.WithTransportCredentials(insecure.NewCredentials())
		logger.Warn("Enrollment using PLAINTEXT gRPC (insecure mode)")
	} else {
		caCert, err := os.ReadFile(cfg.Certs.CAPath)
		if err != nil {
			return fmt.Errorf("read CA certificate for enrollment TLS: %w", err)
		}
		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caCert) {
			return fmt.Errorf("failed to parse CA certificate for enrollment TLS")
		}
		tlsCfg := &tls.Config{
			RootCAs:    caPool,
			MinVersion: tls.VersionTLS12,
		}
		// ServerName override: allows the agent to connect to a custom deployment
		// domain (e.g. "edr.local" or a bare IP) while validating the server cert
		// against the internal service name the cert was actually issued for
		// (e.g. "edr-connection-manager"). This resolves x509 SAN mismatches
		// without requiring re-issuance of server certificates.
		if cfg.Server.TLSServerName != "" {
			tlsCfg.ServerName = cfg.Server.TLSServerName
			logger.Infof("Enrollment TLS: ServerName override → %q (connecting to %s)",
				cfg.Server.TLSServerName, cfg.Server.Address)
		}
		dialOpt = grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg))
		logger.Info("Enrollment using TLS (server-auth only, no client cert)")
	}

	conn, err := grpc.Dial(cfg.Server.Address, dialOpt)
	if err != nil {
		return fmt.Errorf("dial server: %w", err)
	}
	defer conn.Close()

	client := pb.NewEventIngestionServiceClient(conn)
	req := &pb.AgentRegistrationRequest{
		InstallationToken: cfg.Certs.BootstrapToken,
		AgentId:           cfg.Agent.ID,
		Csr:               csrPEM,
		Hostname:          cfg.Agent.Hostname,
		OsType:            "windows",
	}

	ctx, cancel := context.WithTimeout(context.Background(), registerTimeout)
	defer cancel()

	resp, err := client.RegisterAgent(ctx, req)
	if err != nil {
		return fmt.Errorf("register agent: %w", err)
	}

	if resp.GetStatus() != pb.RegistrationStatus_REGISTRATION_STATUS_APPROVED {
		msg := resp.GetMessage()
		if msg == "" {
			msg = "registration not approved"
		}
		return fmt.Errorf("enrollment rejected: %s", msg)
	}

	cert := resp.GetCertificate()
	caChain := resp.GetCaChain()
	if len(cert) == 0 || len(caChain) == 0 {
		return errors.New("server approved but did not return certificate or CA chain")
	}

	if err := cm.SaveCertificate(cert, caChain); err != nil {
		return fmt.Errorf("save certificate: %w", err)
	}

	cfg.Agent.ID = resp.GetAgentId()
	logger.Infof("Enrollment successful; agent ID: %s", cfg.Agent.ID)

	if configFilePath != "" {
		if err := cfg.Save(configFilePath); err != nil {
			return fmt.Errorf("save config after enrollment: %w", err)
		}
		logger.Infof("Config saved to %s", configFilePath)
	}
	return nil
}
