package commands

import (
	"context"
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

		status, exitCode, err := getJobstatus(cmd.Context(), id, jobmanagerpb.NewJobManagerClient(conn))
		if err != nil {
			return err
		}

		fmt.Printf("Status: %s\n", status.String())
		if exitCode != nil {
			fmt.Printf("Exit Code: %d\n", *exitCode)
		}
		return nil
	},
}

func getJobstatus(ctx context.Context, jobId uuid.UUID, client jobmanagerpb.JobManagerClient) (jobmanagerpb.Status, *int32, error) {
	resp, err := client.GetStatus(ctx, &jobmanagerpb.GetStatusRequest{
		JobId: jobId[:],
	})

	if err != nil {
		return 0, nil, fmt.Errorf("server returned error getting job status: %w", err)
	}
	return resp.CurrentStatus, resp.ExitCode, nil
}
