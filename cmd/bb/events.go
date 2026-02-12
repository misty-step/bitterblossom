package main

import (
	"fmt"
	"strings"
	"time"

	storeevents "github.com/misty-step/bitterblossom/internal/events"
	"github.com/spf13/cobra"
)

func newEventsCmd() *cobra.Command {
	var dir string
	var sprites []string
	var kindsRaw []string
	var sinceRaw string
	var untilRaw string
	var issue int
	var jsonMode bool

	cmd := &cobra.Command{
		Use:   "events",
		Short: "Query structured event history from the local event store",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			now := time.Now().UTC()
			start, end, err := parseTimeRange(now, sinceRaw, untilRaw)
			if err != nil {
				return err
			}
			filter, err := buildEventFilter(sprites, kindsRaw, start, end)
			if err != nil {
				return err
			}

			query, err := storeevents.NewQuery(storeevents.QueryConfig{Dir: dir})
			if err != nil {
				return err
			}
			found, err := query.Read(storeevents.QueryOptions{
				Filter: filter,
				Since:  start,
				Until:  end,
				Issue:  issue,
			})
			if err != nil {
				return err
			}

			for _, event := range found {
				if err := writeLogEvent(cmd.OutOrStdout(), event, jsonMode); err != nil {
					return err
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&dir, "dir", storeevents.DefaultDir(), "Event store directory")
	cmd.Flags().StringSliceVar(&sprites, "sprite", nil, "filter by sprite name")
	cmd.Flags().StringSliceVar(&kindsRaw, "type", nil, "filter by event type")
	cmd.Flags().StringVar(&sinceRaw, "since", "", "include events since duration or RFC3339 timestamp")
	cmd.Flags().StringVar(&untilRaw, "until", "", "include events until RFC3339 timestamp")
	cmd.Flags().IntVar(&issue, "issue", 0, "filter by issue number")
	cmd.Flags().BoolVar(&jsonMode, "json", false, "emit JSONL output")

	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		if err == nil {
			return nil
		}
		return fmt.Errorf("events: %s", strings.TrimSpace(err.Error()))
	})

	return cmd
}

