package commands

import (
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

		resp, err := jobmanagerClient.StartJob(cmd.Context(), &jobmanagerpb.StartJobRequest{
			Command: args[0],
			Args:    args[1:],
		})

		if err != nil {
			return fmt.Errorf("server returned error starting job: %w", err)
		}

		var id uuid.UUID
		if id, err = uuid.FromBytes(resp.JobId); err != nil {
			return fmt.Errorf("server returned invalid job id: %w", err)
		} else {
			fmt.Printf("Started Job: %s\n", id.String())
		}
		return nil
	},
}
