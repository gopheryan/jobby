package commands

import (
	"context"
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
		host, _ := cmd.Flags().GetString("host")
		conn, err := newClientConnection(host)
		if err != nil {
			return err
		}
		defer conn.Close()

		var id uuid.UUID
		if id, err = uuid.Parse(args[0]); err != nil {
			return fmt.Errorf("failed to parse job id: %w", err)
		}

		if err := stopJob(cmd.Context(), id, jobmanagerpb.NewJobManagerClient(conn)); err != nil {
			return err
		}
		fmt.Printf("Stopped job %s\n", args[0])
		return nil
	},
}

func stopJob(ctx context.Context, jobId uuid.UUID, client jobmanagerpb.JobManagerClient) error {
	if _, err := client.StopJob(ctx, &jobmanagerpb.StopJobRequest{
		JobId: jobId[:],
	}); err != nil {
		return fmt.Errorf("server returned error stopping job: %w", err)
	}
	return nil
}
