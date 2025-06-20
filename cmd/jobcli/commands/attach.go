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
)

var stdErr bool

func init() {

	attachCmd.Flags().BoolVarP(&stdErr, "stderr", "", false, "attach to stderr output")

	rootCmd.AddCommand(attachCmd)
}

var attachCmd = &cobra.Command{
	Use:  "attach job-id",
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

		outputType := jobmanagerpb.OutputType_OUTPUT_TYPE_STDOUT
		if stdErr {
			outputType = jobmanagerpb.OutputType_OUTPUT_TYPE_STDERR
		}

		return attachJob(cmd.Context(), id, outputType, os.Stdout, jobmanagerpb.NewJobManagerClient(conn))
	},
}

func attachJob(ctx context.Context, jobId uuid.UUID, outputType jobmanagerpb.OutputType, dest io.Writer, jmClient jobmanagerpb.JobManagerClient) error {
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	client, err := jmClient.GetJobOutput(subCtx, &jobmanagerpb.GetJobOutputRequest{
		JobId: jobId[:],
		Type:  outputType,
	})
	if err != nil {
		return fmt.Errorf("server returned error attaching to job output: %w", err)
	}

	var resp *jobmanagerpb.GetJobOutputResponse
	for err == nil {
		resp, err = client.Recv()
		if err == nil {
			if _, err = dest.Write(resp.Data); err != nil {
				return fmt.Errorf("error writing output data to destination: %w", err)
			}
		}
	}

	if !errors.Is(err, io.EOF) {
		return fmt.Errorf("error receiving output data: %w", err)
	}
	return nil
}
