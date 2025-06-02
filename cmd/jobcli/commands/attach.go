package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/google/uuid"
	"github.com/gopheryan/jobby/jobmanagerpb"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var stdErr bool

func init() {
	pflag.BoolVarP(&stdErr, "stderr", "", false, "attach to stderr output")

	rootCmd.AddCommand(attachCmd)
}

var attachCmd = &cobra.Command{
	Use:  "attach job-id",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var id uuid.UUID
		var err error
		if id, err = uuid.Parse(args[0]); err != nil {
			return fmt.Errorf("failed to parse job id: %w", err)
		}

		outputType := jobmanagerpb.OutputType_OUTPUT_TYPE_STDOUT
		if stdErr {
			outputType = jobmanagerpb.OutputType_OUTPUT_TYPE_STDERR
		}

		return attachJob(cmd.Context(), id, outputType, os.Stdout)
	},
}

func attachJob(ctx context.Context, jobId uuid.UUID, outputType jobmanagerpb.OutputType, dest io.Writer) error {
	client, err := jobmanagerClient.GetJobOutput(ctx, &jobmanagerpb.GetJobOutputRequest{
		JobId: jobId[:],
		Type:  outputType,
	})
	if err != nil {
		return fmt.Errorf("server returned error stopping job: %w", err)
	}

	var resp *jobmanagerpb.GetJobOutputResponse
	for err == nil {
		resp, err = client.Recv()
		if err == nil {
			_, err = dest.Write(resp.Data)
		}
	}
	if !errors.Is(err, io.EOF) {
		return fmt.Errorf("error reading output data: %w", err)
	}
	return nil
}
