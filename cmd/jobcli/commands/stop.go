package commands

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/gopheryan/jobby/jobmanagerpb"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(stopCmd)
}

var stopCmd = &cobra.Command{
	Use:  "stop job-id",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var id uuid.UUID
		var err error
		if id, err = uuid.Parse(args[0]); err != nil {
			return fmt.Errorf("failed to parse job id: %w", err)
		}

		_, err = jobmanagerClient.StopJob(cmd.Context(), &jobmanagerpb.StopJobRequest{
			JobId: id[:],
		})

		if err != nil {
			return fmt.Errorf("server returned error stopping job: %w", err)
		}

		fmt.Printf("Stopped job %s\n", args[0])
		return nil
	},
}
