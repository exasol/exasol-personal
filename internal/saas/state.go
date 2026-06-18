// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package saas

import (
	"github.com/exasol/exasol-personal/internal/config"
)

// SaveTarget caches the non-secret resolved SaaS target in deployment.json.
// The connection credential is the SaaS token, so nothing secret is stored here.
func SaveTarget(deployment config.DeploymentDir, target config.DeploymentSaaS) error {
	info, err := config.ReadDeploymentInfo(deployment)
	if err != nil {
		return err
	}

	info.SaaS = &target

	return config.WriteDeploymentInfo(deployment.Root(), info)
}

// SaveAccount persists the SaaS account id and region into deployment.json,
// preserving any other cached SaaS target fields.
func SaveAccount(deployment config.DeploymentDir, accountID, region string) error {
	info, err := config.ReadDeploymentInfo(deployment)
	if err != nil {
		return err
	}
	if info.SaaS == nil {
		info.SaaS = &config.DeploymentSaaS{}
	}
	if accountID != "" {
		info.SaaS.AccountId = accountID
	}
	if region != "" {
		info.SaaS.Region = region
	}

	return config.WriteDeploymentInfo(deployment.Root(), info)
}

// LoadTarget returns the cached SaaS target, or nil when none is cached yet.
func LoadTarget(deployment config.DeploymentDir) (*config.DeploymentSaaS, error) {
	info, err := config.ReadDeploymentInfo(deployment)
	if err != nil {
		return nil, err
	}

	return info.SaaS, nil
}

// FromDatabase builds the resolved target from a SaaS API database plus the
// account context and the chosen database user.
func FromDatabase(accountID, region, username string, database *Database) config.DeploymentSaaS {
	return config.DeploymentSaaS{
		AccountId: accountID,
		Region:    region,
		DbUuid:    database.ID,
		Host:      database.Connection.Host,
		Port:      database.Connection.Port,
		JDBC:      database.Connection.JDBC,
		Username:  username,
	}
}
