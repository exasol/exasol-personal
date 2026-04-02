// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/exasol/exasol-personal/tools/cleanup/internal/aws"
	"github.com/exasol/exasol-personal/tools/cleanup/internal/exoscale"
	"github.com/exasol/exasol-personal/tools/cleanup/internal/shared"
	"github.com/spf13/cobra"
)

var cleanupDiscoverOpts = struct {
	JSON           bool
	OwnerFilter    string
	Order          string
	Legacy         bool
	ownerIsDefault bool
}{}

const cleanupDiscoverShort = "List deployments discovered via Deployment tag"

var cleanupDiscoverCmd = &cobra.Command{
	Use:    "discover",
	Short:  cleanupDiscoverShort,
	Args:   cobra.NoArgs,
	PreRun: func(_ *cobra.Command, _ []string) { configureLogger() },
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		
		var collectors []shared.ProviderCollector
		
		// AWS Collector - default owner to caller identity
		if cleanupOpts.AWSRegion != "" {
			awsOwnerFilter := cleanupDiscoverOpts.OwnerFilter
			
			// AWS-specific default: use caller identity if no filter provided
			if awsOwnerFilter == "" {
				cfg, err := config.LoadDefaultConfig(cmd.Context())
				if err == nil {
					stsClient := sts.NewFromConfig(cfg)
					idOut, err := stsClient.GetCallerIdentity(cmd.Context(), &sts.GetCallerIdentityInput{})
					if err == nil && idOut.Arn != nil && *idOut.Arn != "" {
						awsOwnerFilter = *idOut.Arn
						cleanupDiscoverOpts.ownerIsDefault = true
					}
				}
			}
			
			collectors = append(collectors,
				aws.NewCollector(cleanupOpts.AWSRegion, awsOwnerFilter, cleanupDiscoverOpts.Legacy))
		}
		
		// Exoscale Collector - use provided owner filter or empty for all
		if cleanupOpts.ExoscaleZone != "" {
			exoOwnerFilter := cleanupDiscoverOpts.OwnerFilter
			// Exoscale default: empty means all deployments (no caller identity equivalent)
			
			collectors = append(collectors,
				exoscale.NewCollector(cleanupOpts.ExoscaleZone, exoOwnerFilter, cleanupDiscoverOpts.Legacy))
		}
		
		// Collect from all providers
		res, err := shared.CollectAllProviders(cmd.Context(), collectors)
		if err != nil {
			return err
		}
		// Support comma-separated ordering hierarchy, default to state
		order := cleanupDiscoverOpts.Order
		if order == "" {
			order = "state,created,resources"
		}
		fields := strings.Split(order, ",")
		// normalize and trim spaces
		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}
		valid := map[string]bool{
			"deployment": true,
			"provider":   true,
			"owner":      true,
			"region":     true,
			"created":    true,
			"state":      true,
			"resources":  true,
		}
		sort.Slice(res, func(item1, item2 int) bool {
			for _, f := range fields {
				// support +/- prefix for desc/asc
				dir := 1 // 1 asc, -1 desc
				field := f
				if strings.HasPrefix(field, "+") || strings.HasPrefix(field, "-") {
					if strings.HasPrefix(field, "-") {
						dir = -1
					}
					field = strings.TrimPrefix(strings.TrimPrefix(field, "+"), "-")
				}
				if !valid[field] {
					continue // skip unknown fields silently
				}
				switch field {
				case "deployment":
					if res[item1].ID != res[item2].ID {
						if dir == 1 {
							return res[item1].ID < res[item2].ID
						}

						return res[item1].ID > res[item2].ID
					}
				case "provider":
					if res[item1].Provider != res[item2].Provider {
						if dir == 1 {
							return res[item1].Provider < res[item2].Provider
						}

						return res[item1].Provider > res[item2].Provider
					}
				case "owner":
					if res[item1].Owner != res[item2].Owner {
						if dir == 1 {
							return res[item1].Owner < res[item2].Owner
						}

						return res[item1].Owner > res[item2].Owner
					}
				case "region":
					if res[item1].Region != res[item2].Region {
						if dir == 1 {
							return res[item1].Region < res[item2].Region
						}

						return res[item1].Region > res[item2].Region
					}
				case "created":
					// zero times sort last
					createdItem1 := res[item1].CreatedAt
					createdItem2 := res[item2].CreatedAt
					if createdItem1.IsZero() != createdItem2.IsZero() {
						if dir == 1 {
							return !createdItem1.IsZero() && createdItem2.IsZero()
						}

						return createdItem1.IsZero() && !createdItem2.IsZero()
					}
					if !createdItem1.Equal(createdItem2) {
						if dir == 1 {
							return createdItem1.Before(createdItem2)
						}

						return createdItem2.Before(createdItem1)
					}
				case "state":
					if res[item1].State != res[item2].State {
						if dir == 1 {
							return res[item1].State < res[item2].State
						}

						return res[item1].State > res[item2].State
					}
				case "resources":
					if res[item1].Resources != res[item2].Resources {
						if dir == 1 {
							return res[item1].Resources > res[item2].Resources
						}

						return res[item1].Resources < res[item2].Resources
					}
				default:
					// unknown field: keep current order by not changing comparison
					// no-op
				}
			}
			// final tie-breaker: deployment id
			return res[item1].ID < res[item2].ID
		})
		if cleanupDiscoverOpts.JSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")

			return enc.Encode(res)
		}

		// Prepare table rows
		if len(res) > 0 {
			rows := make([][]string, 0, len(res))
			for _, deployment := range res {
				created := "-"
				if !deployment.CreatedAt.IsZero() {
					created = deployment.CreatedAt.Format("2006-01-02 15:04")
				}
				owner := deployment.Owner
				if owner == "" {
					owner = "-"
				}
				rows = append(
					rows, []string{
						deployment.ID,
						deployment.Provider,
						deployment.Region,
						owner,
						created,
						deployment.State,
						strconv.Itoa(deployment.Resources),
					},
				)
			}
			shared.RenderTable(
				cmd.OutOrStdout(),
				[]string{"deployment", "provider", "region", "owner", "created", "state", "resources"},
				[]int{20, 10, 14, 40, 22, 10, 9},
				rows,
			)

			fmt.Fprintf(cmd.OutOrStdout(), "\n")
		}

		// Build a concise summary of search parameters
		mode := "project=exasol-personal"
		if cleanupDiscoverOpts.Legacy {
			mode = "legacy"
		}
		regionDisplay := cleanupOpts.AWSRegion
		if regionDisplay == "" {
			regionDisplay = "(default)"
		}
		if _, err := fmt.Fprintf(
			cmd.OutOrStdout(),
			"Total: %d deployments (owner=%s, region=%s, mode=%s, order=%s)\n",
			len(res),
			ownerFilterDisplay(cleanupDiscoverOpts.OwnerFilter),
			regionDisplay,
			mode,
			order,
		); err != nil {
			// non-fatal: log to stderr-like output
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "write output failed:", err)
		}
		slog.Info("discover complete", "count", len(res))

		return nil
	},
}

func registerCleanupDiscoverFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&cleanupDiscoverOpts.JSON, "json", false, "Output JSON")
	cmd.Flags().
		StringVar(&cleanupDiscoverOpts.OwnerFilter, "owner", "",
			"Owner ARN/wildcard to filter (defaults to caller; use * for any)")
	cmd.Flags().StringVar(&cleanupDiscoverOpts.Order, "order", "state,created,resources",
		"Order by columns (comma-separated). Prefix +/- for asc/desc per field."+
			" Fields: deployment,provider,owner,region,created,state,resources."+
			" Default: state,created,resources")
	cmd.Flags().BoolVar(&cleanupDiscoverOpts.Legacy, "legacy", false,
		"Discover legacy deployments (ignore mandatory Project=exasol-personal)")
}

func ownerFilterDisplay(f string) string {
	if cleanupDiscoverOpts.ownerIsDefault {
		return "(caller)"
	}

	if f == "" {
		return "(caller)"
	}

	return f
}

// nolint: gochecknoinits
func init() {
	rootCmd.AddCommand(cleanupDiscoverCmd)
	registerCleanupDiscoverFlags(cleanupDiscoverCmd)
}
