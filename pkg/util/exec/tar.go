package exec

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const (
	tarDirPermission  = 0o755
	tarFilePermission = 0o644

	// maxTrailingTarBytes is the maximum number of trailing bytes we drain from
	// the pipe after tar extraction finishes. This prevents a compromised or
	// misbehaving pod from sending unbounded data after the tar end marker.
	maxTrailingTarBytes int64 = 1 << 20 // 1 MiB

	msgSkippingSymlink      = "skipping symlink during tar creation"
	msgSkippingTarEntryType = "skipping unsupported tar entry type"
)

// CopyOptions configures a file copy between a local path and a pod.
type CopyOptions struct {
	Namespace     string
	PodName       string
	ContainerName string
	PodPath       string
	LocalPath     string
}

// CopyFromPod copies files from a pod directory to a local directory using tar.
func CopyFromPod(ctx context.Context, executor Executor, opts CopyOptions) error {
	reader, writer := io.Pipe()

	errCh := make(chan error, 1)

	go func() {
		defer func() { _ = writer.Close() }()

		errCh <- executor.Exec(ctx, ExecOptions{
			Namespace:     opts.Namespace,
			PodName:       opts.PodName,
			ContainerName: opts.ContainerName,
			Command:       []string{"tar", "cf", "-", "-C", opts.PodPath, "--exclude=lost+found", "."},
			Stdout:        writer,
			Stderr:        io.Discard,
		})
	}()

	extractErr := extractTar(reader, opts.LocalPath)
	if extractErr == nil {
		// Drain remaining bytes so SPDY executor goroutines finish cleanly
		// before we close the pipe. Use a bounded reader to guard against a
		// compromised pod streaming data indefinitely after the tar end marker.
		n, _ := io.Copy(io.Discard, io.LimitReader(reader, maxTrailingTarBytes+1))
		if n > maxTrailingTarBytes {
			extractErr = fmt.Errorf("trailing data after tar archive exceeds %d bytes", maxTrailingTarBytes)
		}
	}
	_ = reader.CloseWithError(extractErr)

	execErr := <-errCh

	if extractErr != nil {
		return fmt.Errorf("extracting tar from pod: %w", extractErr)
	}

	if execErr != nil {
		return fmt.Errorf("tar exec in pod: %w", execErr)
	}

	return nil
}

// CopyToPod copies files from a local directory to a pod directory using tar.
func CopyToPod(ctx context.Context, executor Executor, opts CopyOptions) error {
	reader, writer := io.Pipe()

	errCh := make(chan error, 1)

	go func() {
		defer func() { _ = writer.Close() }()
		errCh <- createTar(writer, opts.LocalPath)
	}()

	execErr := executor.Exec(ctx, ExecOptions{
		Namespace:     opts.Namespace,
		PodName:       opts.PodName,
		ContainerName: opts.ContainerName,
		Command:       []string{"tar", "xf", "-", "-C", opts.PodPath},
		Stdin:         reader,
		Stderr:        io.Discard,
	})
	// Close the read end so the producer goroutine unblocks if it is still writing.
	_ = reader.CloseWithError(execErr)

	tarErr := <-errCh

	if execErr != nil {
		return fmt.Errorf("tar exec in pod: %w", execErr)
	}

	if tarErr != nil {
		return fmt.Errorf("creating tar archive: %w", tarErr)
	}

	return nil
}

func extractTar(r io.Reader, destDir string) error {
	tr := tar.NewReader(r)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}

		if err != nil {
			return fmt.Errorf("reading tar header: %w", err)
		}

		target := filepath.Join(destDir, filepath.Clean(header.Name))

		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) && target != filepath.Clean(destDir) {
			return fmt.Errorf("tar entry %q escapes destination directory", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, tarDirPermission); err != nil {
				return fmt.Errorf("creating directory %s: %w", target, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), tarDirPermission); err != nil {
				return fmt.Errorf("creating parent directory: %w", err)
			}

			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, tarFilePermission) //nolint:gosec // Path validated above against directory traversal.
			if err != nil {
				return fmt.Errorf("creating file %s: %w", target, err)
			}

			if _, err := io.Copy(f, tr); err != nil { //nolint:gosec // Trusted source, bounded by pod filesystem.
				_ = f.Close()

				return fmt.Errorf("writing file %s: %w", target, err)
			}

			if err := f.Close(); err != nil {
				return fmt.Errorf("closing file %s: %w", target, err)
			}
		default:
			slog.Info(msgSkippingTarEntryType, "type", header.Typeflag, "name", header.Name)
		}
	}
}

func createTar(w io.Writer, srcDir string) error {
	tw := tar.NewWriter(w)
	defer func() { _ = tw.Close() }()

	if err := filepath.Walk(srcDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return fmt.Errorf("computing relative path: %w", err)
		}

		if relPath == "." {
			return nil
		}

		if info.IsDir() && info.Name() == "lost+found" {
			return filepath.SkipDir
		}

		// Symlinks cannot be followed safely inside a tar archive because
		// their targets may not exist on the receiving end. Skip them and
		// log so operators can investigate if needed.
		if info.Mode()&os.ModeSymlink != 0 {
			slog.Info(msgSkippingSymlink, "path", path)

			return nil
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("creating tar header: %w", err)
		}

		header.Name = relPath

		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("writing tar header: %w", err)
		}

		if info.IsDir() {
			return nil
		}

		f, err := os.Open(path) //nolint:gosec // Path comes from trusted local Walk, not user input.
		if err != nil {
			return fmt.Errorf("opening file %s: %w", path, err)
		}
		defer func() { _ = f.Close() }()

		if _, err := io.Copy(tw, f); err != nil {
			return fmt.Errorf("writing file %s to tar: %w", path, err)
		}

		return nil
	}); err != nil {
		return fmt.Errorf("walking source directory: %w", err)
	}

	return nil
}
