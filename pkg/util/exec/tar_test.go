package exec_test

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opendatahub-io/odh-cli/pkg/util/exec"

	. "github.com/onsi/gomega"
)

const (
	testFileName    = "hello.txt"
	testFileContent = "hello world"
	testDirName     = "subdir"
)

func TestCopyFromPod_EmptyTar(t *testing.T) {
	g := NewWithT(t)

	destDir := t.TempDir()

	executor := &exec.MockExecutor{
		ExecFn: func(_ context.Context, opts exec.ExecOptions) error {
			tw := tar.NewWriter(opts.Stdout)
			_ = tw.Close()

			// Write trailing bytes that the drain logic should absorb.
			_, _ = opts.Stdout.Write([]byte{0, 0, 0, 0})

			return nil
		},
	}

	err := exec.CopyFromPod(t.Context(), executor, exec.CopyOptions{
		Namespace:     "ns",
		PodName:       "pod",
		ContainerName: "ctr",
		PodPath:       "/data",
		LocalPath:     destDir,
	})

	g.Expect(err).ToNot(HaveOccurred())

	entries, err := os.ReadDir(destDir)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(entries).To(BeEmpty())
}

func TestCopyFromPod_WithFiles(t *testing.T) {
	g := NewWithT(t)

	destDir := t.TempDir()

	executor := &exec.MockExecutor{
		ExecFn: func(_ context.Context, opts exec.ExecOptions) error {
			tw := tar.NewWriter(opts.Stdout)

			content := []byte(testFileContent)
			if err := tw.WriteHeader(&tar.Header{
				Name: testFileName,
				Mode: 0o644,
				Size: int64(len(content)),
			}); err != nil {
				return fmt.Errorf("writing tar header: %w", err)
			}
			if _, err := tw.Write(content); err != nil {
				return fmt.Errorf("writing tar content: %w", err)
			}

			return tw.Close()
		},
	}

	err := exec.CopyFromPod(t.Context(), executor, exec.CopyOptions{
		Namespace:     "ns",
		PodName:       "pod",
		ContainerName: "ctr",
		PodPath:       "/data",
		LocalPath:     destDir,
	})

	g.Expect(err).ToNot(HaveOccurred())

	data, err := os.ReadFile(filepath.Join(destDir, testFileName)) //nolint:gosec // Test file path constructed from t.TempDir, not user input.
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(string(data)).To(Equal(testFileContent))
}

func TestCopyFromPod_ExtractionError(t *testing.T) {
	g := NewWithT(t)

	destDir := t.TempDir()

	executor := &exec.MockExecutor{
		ExecFn: func(_ context.Context, opts exec.ExecOptions) error {
			_, _ = opts.Stdout.Write([]byte("not a valid tar stream"))

			return nil
		},
	}

	err := exec.CopyFromPod(t.Context(), executor, exec.CopyOptions{
		Namespace:     "ns",
		PodName:       "pod",
		ContainerName: "ctr",
		PodPath:       "/data",
		LocalPath:     destDir,
	})

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("extracting tar from pod"))
}

func TestCopyFromPod_DrainsPipeBeforeClose(t *testing.T) {
	g := NewWithT(t)

	destDir := t.TempDir()

	pipeWriteErrCh := make(chan error, 1)

	executor := &exec.MockExecutor{
		ExecFn: func(_ context.Context, opts exec.ExecOptions) error {
			tw := tar.NewWriter(opts.Stdout)
			_ = tw.Close()

			// Allow CopyFromPod to finish tar extraction and start the drain
			// loop. The io.Pipe guarantees ordering: this Write blocks until the
			// reader is actively draining, so the sleep only needs to cover the
			// scheduling gap between extraction and drain start.
			time.Sleep(10 * time.Millisecond)

			trailing := bytes.Repeat([]byte{0}, 512)
			_, err := opts.Stdout.Write(trailing)
			pipeWriteErrCh <- err

			return nil
		},
	}

	err := exec.CopyFromPod(t.Context(), executor, exec.CopyOptions{
		Namespace:     "ns",
		PodName:       "pod",
		ContainerName: "ctr",
		PodPath:       "/data",
		LocalPath:     destDir,
	})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(<-pipeWriteErrCh).ToNot(HaveOccurred())
}

func TestCopyFromPod_ExcessTrailingData(t *testing.T) {
	g := NewWithT(t)

	destDir := t.TempDir()

	executor := &exec.MockExecutor{
		ExecFn: func(_ context.Context, opts exec.ExecOptions) error {
			tw := tar.NewWriter(opts.Stdout)
			_ = tw.Close()

			trailing := bytes.Repeat([]byte{0}, (1<<20)+1)
			_, _ = opts.Stdout.Write(trailing)

			return nil
		},
	}

	err := exec.CopyFromPod(t.Context(), executor, exec.CopyOptions{
		Namespace:     "ns",
		PodName:       "pod",
		ContainerName: "ctr",
		PodPath:       "/data",
		LocalPath:     destDir,
	})

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("trailing data after tar archive"))
}

func TestCopyToPod(t *testing.T) {
	g := NewWithT(t)

	srcDir := t.TempDir()

	err := os.MkdirAll(filepath.Join(srcDir, testDirName), 0o750)
	g.Expect(err).ToNot(HaveOccurred())

	err = os.WriteFile(filepath.Join(srcDir, testFileName), []byte(testFileContent), 0o600)
	g.Expect(err).ToNot(HaveOccurred())

	var received bytes.Buffer
	executor := &exec.MockExecutor{
		ExecFn: func(_ context.Context, opts exec.ExecOptions) error {
			if _, err := io.Copy(&received, opts.Stdin); err != nil {
				return fmt.Errorf("copying stdin: %w", err)
			}

			return nil
		},
	}

	err = exec.CopyToPod(t.Context(), executor, exec.CopyOptions{
		Namespace:     "ns",
		PodName:       "pod",
		ContainerName: "ctr",
		PodPath:       "/data",
		LocalPath:     srcDir,
	})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(received.Len()).To(BeNumerically(">", 0))

	tr := tar.NewReader(&received)
	names := make([]string, 0)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		g.Expect(err).ToNot(HaveOccurred())
		names = append(names, header.Name)
	}

	g.Expect(names).To(ContainElement(testFileName))
	g.Expect(names).To(ContainElement(testDirName))
}

func TestCopyFromPod_DirectoryTraversal(t *testing.T) {
	g := NewWithT(t)

	destDir := t.TempDir()

	executor := &exec.MockExecutor{
		ExecFn: func(_ context.Context, opts exec.ExecOptions) error {
			tw := tar.NewWriter(opts.Stdout)

			if err := tw.WriteHeader(&tar.Header{
				Name: "../../etc/passwd",
				Mode: 0o644,
				Size: 4,
			}); err != nil {
				return fmt.Errorf("writing tar header: %w", err)
			}

			if _, err := tw.Write([]byte("evil")); err != nil {
				return fmt.Errorf("writing tar content: %w", err)
			}

			return tw.Close()
		},
	}

	err := exec.CopyFromPod(t.Context(), executor, exec.CopyOptions{
		Namespace:     "ns",
		PodName:       "pod",
		ContainerName: "ctr",
		PodPath:       "/data",
		LocalPath:     destDir,
	})

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("escapes destination directory"))
}
