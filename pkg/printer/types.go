package printer

import "k8s.io/cli-runtime/pkg/genericiooptions"

type Options struct {
	IOStreams    genericiooptions.IOStreams
	OutputFormat string
}
