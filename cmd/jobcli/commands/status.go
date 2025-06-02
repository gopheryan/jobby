package commands

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/gopheryan/jobby/jobmanagerpb"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(statusCmd)
}

var statusCmd = &cobra.Command{
	Use:  "status job-id",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var id uuid.UUID
		var err error
		if id, err = uuid.Parse(args[0]); err != nil {
			return fmt.Errorf("failed to parse job id: %w", err)
		}

		resp, err := jobmanagerClient.GetStatus(cmd.Context(), &jobmanagerpb.GetStatusRequest{
			JobId: id[:],
		})

		if err != nil {
			return fmt.Errorf("server returned error getting job status: %w", err)
		}

		fmt.Printf("Status: %s\n", resp.CurrentStatus.String())
		if resp.ExitCode != nil {
			fmt.Printf("Exit Code: %d\n", *resp.ExitCode)
		}
		return nil
	},
}
