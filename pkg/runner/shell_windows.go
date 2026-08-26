//go:build windows

package runner

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"os"
	"os/exec"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func shellQuote(value string) string {
	value = escapePercentExpansion(value)
	if value != "" && strings.IndexFunc(value, func(char rune) bool {
		return !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_@%+=:,./-\\", char)
	}) < 0 {
		return value
	}

	var quoted strings.Builder
	quoted.WriteByte('"')
	for {
		quote := strings.IndexByte(value, '"')
		if quote < 0 {
			writeCmdQuotedSegment(&quoted, value)
			break
		}
		writeCmdQuotedSegment(&quoted, value[:quote])
		// Close the current quoted segment, emit a caret-escaped literal quote,
		// then reopen quoting before any more untrusted argument data.
		quoted.WriteString(`"\^""`)
		value = value[quote+1:]
	}
	quoted.WriteByte('"')
	return quoted.String()
}

func writeCmdQuotedSegment(quoted *strings.Builder, segment string) {
	trailingBackslashes := len(segment) - len(strings.TrimRight(segment, "\\"))
	quoted.WriteString(segment)
	quoted.WriteString(strings.Repeat("\\", trailingBackslashes))
}

func curlShellCommand(command, body string) (string, []string, error) {
	if body == "" {
		return command + "\r\nexit /b %errorlevel%", nil, nil
	}

	bodyFile, err := os.CreateTemp("", "apih-curl-body-")
	if err != nil {
		return "", nil, err
	}
	bodyPath := bodyFile.Name()
	if err := bodyFile.Close(); err != nil {
		removeFiles([]string{bodyPath})
		return "", nil, err
	}
	if err := os.Remove(bodyPath); err != nil {
		removeFiles([]string{bodyPath})
		return "", nil, err
	}
	encodedPath := bodyPath + ".b64"
	temporaryPaths := []string{encodedPath, bodyPath}
	encoded := base64.StdEncoding.EncodeToString([]byte(body))

	lines := make([]string, 0, len(encoded)/4096+8)
	for start := 0; start < len(encoded); start += 4096 {
		end := min(start+4096, len(encoded))
		redirect := ">"
		if start > 0 {
			redirect = ">>"
		}
		lines = append(lines, redirect+shellQuote(encodedPath)+" echo "+encoded[start:end])
	}
	lines = append(lines,
		"certutil -f -decode "+shellQuote(encodedPath)+" "+shellQuote(bodyPath)+" >nul",
		"if errorlevel 1 exit /b %errorlevel%",
		"del /q "+shellQuote(encodedPath),
		"type "+shellQuote(bodyPath)+" | "+command,
		`set "apih_curl_exit=%errorlevel%"`,
		"del /q "+shellQuote(bodyPath),
		"exit /b %apih_curl_exit%",
	)
	return strings.Join(lines, "\r\n"), temporaryPaths, nil
}

func executeCurlShellCommand(ctx context.Context, command string) (string, string, int, error) {
	if err := ctx.Err(); err != nil {
		return "", "", -1, err
	}

	// Keep cmd.exe and every descendant in one cancellable process tree.
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return "", "", -1, err
	}
	defer windows.CloseHandle(job)

	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)),
		uint32(unsafe.Sizeof(limits)),
	); err != nil {
		return "", "", -1, err
	}

	cmd := exec.Command("cmd.exe", "/d", "/q", "/v:off")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", "", -1, err
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return "", stderr.String(), -1, err
	}
	process, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(cmd.Process.Pid),
	)
	if err == nil {
		err = windows.AssignProcessToJobObject(job, process)
		_ = windows.CloseHandle(process)
	}
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return "", stderr.String(), -1, err
	}

	stopCancellationWatcher := make(chan struct{})
	cancellationWatcherDone := make(chan struct{})
	go func() {
		defer close(cancellationWatcherDone)
		select {
		case <-ctx.Done():
			_ = windows.TerminateJobObject(job, uint32(windows.ERROR_OPERATION_ABORTED))
		case <-stopCancellationWatcher:
		}
	}()

	_, writeErr := io.WriteString(stdin, command)
	closeErr := stdin.Close()
	waitErr := cmd.Wait()
	close(stopCancellationWatcher)
	<-cancellationWatcherDone

	err = waitErr
	if err == nil {
		err = writeErr
	}
	if err == nil {
		err = closeErr
	}
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	if err != nil && cmd.ProcessState == nil {
		exitCode = -1
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return stdout.String(), stderr.String(), exitCode, err
}

func escapePercentExpansion(value string) string {
	var escaped strings.Builder
	start := 0
	for {
		opening := strings.IndexByte(value[start:], '%')
		if opening < 0 {
			escaped.WriteString(value[start:])
			return escaped.String()
		}
		opening += start
		closing := strings.IndexByte(value[opening+1:], '%')
		if closing < 0 {
			escaped.WriteString(value[start:])
			return escaped.String()
		}
		closing += opening + 1
		escaped.WriteString(value[start:closing])
		escaped.WriteByte('^')
		escaped.WriteByte('%')
		start = closing + 1
	}
}
