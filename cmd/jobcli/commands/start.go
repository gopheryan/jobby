package commands

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/gopheryan/jobby/jobmanagerpb"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(startCmd)
}

var startCmd = &cobra.Command{
	Use:  "start command [arg] ...",
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		host, _ := cmd.Flags().GetString("host")
		conn, err := newClientConnection(host)
		if err != nil {
			return err
		}
		defer conn.Close()

		jobId, err := startJob(cmd.Context(), args[0], args[1:], jobmanagerpb.NewJobManagerClient(conn))
		if err != nil {
			return err
		}
		fmt.Printf("Started Job: %s\n", jobId.String())
		return nil
	},
}

func startJob(ctx context.Context, command string, args []string, client jobmanagerpb.JobManagerClient) (uuid.UUID, error) {
	resp, err := client.StartJob(ctx, &jobmanagerpb.StartJobRequest{
		Command: command,
		Args:    args,
	})

	if err != nil {
		return uuid.UUID{}, fmt.Errorf("server returned error starting job: %w", err)
	}

	var id uuid.UUID
	if id, err = uuid.FromBytes(resp.JobId); err != nil {
		return uuid.UUID{}, fmt.Errorf("server returned invalid job id: %w", err)
	} else {
		return id, nil
	}
}
