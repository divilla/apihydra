#!/usr/bin/env perl
use strict;
use warnings;

use Cwd qw(abs_path getcwd);
use Errno qw(EINTR);
use File::Basename qw(dirname);
use File::Spec;
use File::Temp qw(tempdir);
use IO::Handle;
use IO::Select;
use POSIX qw(WIFEXITED WIFSIGNALED WEXITSTATUS WTERMSIG setpgid);
use Time::HiRes qw(CLOCK_MONOTONIC clock_gettime);

STDOUT->autoflush(1);
STDERR->autoflush(1);

my @ACTIVITY_FRAMES = ("\xC2\xB7", "\xE2\x80\xA2", "\xE2\x97\x8F", "\xE2\x80\xA2");
my $SUCCESS_SYMBOL = "\xE2\x9C\x85";
my $FAILURE_SYMBOL = "\xE2\x9D\x8C";
my $ACTIVITY_INTERVAL = 0.25;
my $OUTPUT_DOT_INTERVAL = 1;

my $active_codex_pid;
my $temporary_dir;
my $active_output_log;
my $result_file;
my $requested_exit_code = 0;

sub fail {
	my ($message) = @_;
	print STDERR "codex-implement: $message\n";
	exit 1;
}

sub read_file {
	my ($path) = @_;
	open my $handle, '<:raw', $path or fail("cannot read $path: $!");
	local $/;
	my $contents = <$handle>;
	close $handle or fail("cannot close $path: $!");
	return defined $contents ? $contents : '';
}

sub shell_quote {
	my ($argument) = @_;
	return $argument if $argument =~ m{\A[a-zA-Z0-9_\@%+=:,./-]+\z};
	$argument =~ s/'/'\\''/g;
	return "'$argument'";
}

sub shell_join {
	return join ' ', map { shell_quote($_) } @_;
}

sub command_exit_code {
	my ($status) = @_;
	return WEXITSTATUS($status) if WIFEXITED($status);
	return 128 + WTERMSIG($status) if WIFSIGNALED($status);
	return 1;
}

sub capture_command {
	my ($quiet_stderr, @command) = @_;
	pipe my $reader, my $writer or fail("cannot create command pipe: $!");
	my $pid = fork();
	defined $pid or fail("cannot fork @command: $!");
	if ($pid == 0) {
		close $reader;
		open STDOUT, '>&', $writer or exit 127;
		if ($quiet_stderr) {
			open STDERR, '>', File::Spec->devnull() or exit 127;
		}
		close $writer;
		exec { $command[0] } @command;
		exit 127;
	}
	close $writer;
	local $/;
	my $output = <$reader>;
	close $reader;
	waitpid $pid, 0;
	return (defined $output ? $output : '', command_exit_code($?));
}

sub run_command {
	my (@command) = @_;
	system { $command[0] } @command;
	return command_exit_code($?);
}

sub run_checked {
	my (@command) = @_;
	my $status = run_command(@command);
	exit $status if $status != 0;
}

sub command_exists {
	my ($command) = @_;
	for my $directory (File::Spec->path()) {
		my $candidate = File::Spec->catfile($directory, $command);
		return 1 if -f $candidate && -x $candidate;
	}
	return 0;
}

sub terminal_width {
	my $width = $ENV{COLUMNS} // 120;
	if (-t STDOUT) {
		my ($detected, $status) = capture_command(1, 'tput', 'cols');
		$detected =~ s/\s+\z//;
		$width = $detected if $status == 0 && $detected =~ /\A[1-9][0-9]*\z/;
	}
	return $width =~ /\A[1-9][0-9]*\z/ ? $width : 120;
}

sub print_command {
	my $separator = '-' x terminal_width();
	print "$separator\n", shell_join(@_), "\n$separator\n";
}

sub monotonic_time {
	return clock_gettime(CLOCK_MONOTONIC);
}

sub new_progress_state {
	my ($label, $terminal, $output) = @_;
	return {
		active       => 1,
		dots         => 0,
		label        => $label,
		last_dot_at  => undef,
		output       => $output,
		started_at   => monotonic_time(),
		terminal     => $terminal,
	};
}

sub progress_text {
	my ($progress, $now) = @_;
	$now //= monotonic_time();
	my $elapsed = int($now - $progress->{started_at});
	return sprintf '%s %02d:%02d %s',
		$progress->{label}, int($elapsed / 60), $elapsed % 60, '.' x $progress->{dots};
}

sub activity_frame {
	my ($elapsed) = @_;
	my $frame_index = int($elapsed / $ACTIVITY_INTERVAL) % scalar @ACTIVITY_FRAMES;
	return $ACTIVITY_FRAMES[$frame_index];
}

sub record_output {
	my ($progress, $now) = @_;
	$now //= monotonic_time();
	if (!defined $progress->{last_dot_at} || $now - $progress->{last_dot_at} >= $OUTPUT_DOT_INTERVAL) {
		$progress->{dots}++;
		$progress->{last_dot_at} = $now;
	}
}

sub render_progress {
	my ($progress, $now) = @_;
	return if !$progress->{terminal};
	$now //= monotonic_time();
	my $frame = activity_frame($now - $progress->{started_at});
	print { $progress->{output} } "\r\033[2K", progress_text($progress, $now), " $frame";
}

sub finish_progress {
	my ($progress, $success) = @_;
	return if !$progress->{active};
	my $symbol = $success ? $SUCCESS_SYMBOL : $FAILURE_SYMBOL;
	my $prefix = $progress->{terminal} ? "\r\033[2K" : '';
	print { $progress->{output} } $prefix, progress_text($progress), " $symbol\n";
	$progress->{active} = 0;
}

sub stop {
	my ($exit_code) = @_;
	$requested_exit_code = $exit_code;
	$SIG{INT} = 'DEFAULT';
	$SIG{TERM} = 'DEFAULT';
	if (defined $active_codex_pid) {
		kill 'TERM', -$active_codex_pid;
		return;
	}
	print STDERR "codex-implement: interrupted\n";
	exit $requested_exit_code;
}

sub start_command {
	my (@command) = @_;
	pipe my $reader, my $writer or fail("cannot create Codex output pipe: $!");
	my $pid = fork();
	defined $pid or fail("cannot fork Codex command: $!");
	if ($pid == 0) {
		setpgid(0, 0) or exit 127;
		close $reader;
		open STDOUT, '>&', $writer or exit 127;
		open STDERR, '>&', $writer or exit 127;
		close $writer;
		exec { $command[0] } @command;
		exit 127;
	}
	close $writer;
	setpgid($pid, $pid);
	return ($pid, $reader);
}

sub monitor_command {
	my ($progress, $output_log, @command) = @_;
	my ($pid, $reader) = start_command(@command);
	$active_codex_pid = $pid;
	my $selector = IO::Select->new($reader);
	my $next_render_at = $progress->{started_at};
	render_progress($progress, $next_render_at);

	open my $log, '>:raw', $output_log or fail("cannot create $output_log: $!");
	while ($selector->count) {
		my $now = monotonic_time();
		if ($now >= $next_render_at) {
			render_progress($progress, $now);
			$next_render_at += $ACTIVITY_INTERVAL while $next_render_at <= $now;
		}
		my $timeout = $next_render_at - monotonic_time();
		$timeout = 0 if $timeout < 0;
		my @ready = $selector->can_read($timeout);
		next if !@ready;

		my $buffer = '';
		my $bytes = sysread $reader, $buffer, 64 * 1024;
		if (!defined $bytes) {
			next if $! == EINTR;
			close $log;
			fail("cannot read Codex output: $!");
		}
		if ($bytes == 0) {
			$selector->remove($reader);
			close $reader;
			next;
		}
		print {$log} $buffer or fail("cannot write $output_log: $!");
		record_output($progress);
	}
	close $log or fail("cannot close $output_log: $!");

	waitpid $pid, 0;
	my $exit_code = command_exit_code($?);
	$active_codex_pid = undef;
	return $exit_code;
}

sub run_codex {
	my (@command) = @_;
	print_command(@command);
	my $progress = new_progress_state('Implement', -t STDOUT, \*STDOUT);
	$active_output_log = File::Spec->catfile($temporary_dir, 'codex-output.log');
	my $exit_code = monitor_command($progress, $active_output_log, @command);
	if ($requested_exit_code != 0) {
		finish_progress($progress, 0);
		unlink $active_output_log;
		$active_output_log = undef;
		print STDERR "codex-implement: interrupted\n";
		exit $requested_exit_code;
	}

	finish_progress($progress, $exit_code == 0);
	if ($exit_code != 0) {
		print STDERR "codex-implement: Codex command failed with exit code $exit_code\n";
		if (-s $active_output_log) {
			my $output = read_file($active_output_log);
			$output =~ s/^/  /mg;
			print STDERR "codex-implement: Codex command output:\n$output";
		}
	}
	unlink $active_output_log;
	$active_output_log = undef;
	return $exit_code;
}

sub path_is_inside {
	my ($path, $parent) = @_;
	return 1 if $path eq $parent;
	return index($path, $parent . '/') == 0;
}

sub select_temp_root {
	my ($repo_root_physical) = @_;
	for my $candidate ($ENV{TMPDIR}, $ENV{TMP}, $ENV{TEMP}, '/tmp') {
		next if !defined $candidate || $candidate eq '' || !-d $candidate || !-w $candidate;
		my $resolved = abs_path($candidate);
		next if !defined $resolved || path_is_inside($resolved, $repo_root_physical);
		return $resolved;
	}
	return;
}

sub ensure_clean_worktree {
	my ($changes, $status) = capture_command(0, 'git', 'status', '--porcelain=v1', '--untracked-files=all', '--', '.');
	$status == 0 or exit $status;
	$changes eq '' or fail('working tree must be clean');
}

sub checkout_change_branch {
	my ($branch) = @_;
	my (undef, $local_status) = capture_command(1, 'git', 'show-ref', '--verify', '--quiet', "refs/heads/$branch");
	if ($local_status == 0) {
		run_checked('git', 'checkout', $branch);
		return;
	}
	$local_status == 1 or fail("cannot inspect branch $branch");

	my (undef, $remote_status) = capture_command(1, 'git', 'show-ref', '--verify', '--quiet', "refs/remotes/origin/$branch");
	if ($remote_status == 0) {
		run_checked('git', 'checkout', '--track', '-b', $branch, "origin/$branch");
		return;
	}
	$remote_status == 1 or fail("cannot inspect origin/$branch");
	run_checked('git', 'checkout', '-b', $branch);
}

sub cleanup {
	unlink $active_output_log if defined $active_output_log && -e $active_output_log;
	unlink $result_file if defined $result_file && -e $result_file;
	rmdir $temporary_dir if defined $temporary_dir && -d $temporary_dir;
}

sub main {
	my (@arguments) = @_;
	@arguments == 1 or fail('usage: codex-implement.pl SPECIFICATION');
	command_exists('codex') or fail('codex is not installed or is not on PATH');
	command_exists('git') or fail('git is not installed or is not on PATH');

	my $script_dir = abs_path(dirname($0));
	my ($repo_root, $repo_status) = capture_command(1, 'git', '-C', $script_dir, 'rev-parse', '--show-toplevel');
	$repo_status == 0 or fail('scripts directory is not inside a git repository');
	$repo_root =~ s/\s+\z//;
	chdir $repo_root or fail("cannot enter repository $repo_root: $!");
	my $repo_root_physical = abs_path(getcwd());

	my $specification = $arguments[0];
	-f $specification or fail("specification file not found: $specification");
	my ($change_name) = $specification =~ m{/([0-9]+-[^./]+)\.md\z}
		or fail('specification path must end with /<number>-<name>.md');
	my $branch = "change/$change_name";
	my ($valid_branch, $valid_status) = capture_command(1, 'git', 'check-ref-format', '--branch', $branch);
	$valid_branch =~ s/\s+\z//;
	$valid_status == 0 && $valid_branch eq $branch or fail("invalid change branch derived from specification: $branch");

	ensure_clean_worktree();
	checkout_change_branch($branch);
	ensure_clean_worktree();

	my ($before_implementation, $head_status) = capture_command(0, 'git', 'rev-parse', 'HEAD');
	$head_status == 0 or exit $head_status;
	$before_implementation =~ s/\s+\z//;

	my $temp_root = select_temp_root($repo_root_physical);
	defined $temp_root or fail('cannot find a writable temporary directory outside the repository');
	$temporary_dir = tempdir('apih-implement.XXXXXXXXXX', DIR => $temp_root, CLEANUP => 0);
	defined $temporary_dir or fail("cannot create private implementation directory under $temp_root");
	$result_file = File::Spec->catfile($temporary_dir, 'implementation-result.md');

	print "Repository: $repo_root\n";
	print "Branch: $branch\n";
	print "Specification: $specification\n";
	my $prompt = '$change-code ' . $specification;
	my $status = run_codex('codex', 'exec', '--json', '-o', $result_file, $prompt);
	exit $status if $status != 0;
	-f $result_file or fail('implementation did not write a final response');

	my ($after_implementation, $after_status) = capture_command(0, 'git', 'rev-parse', 'HEAD');
	$after_status == 0 or exit $after_status;
	$after_implementation =~ s/\s+\z//;
	$after_implementation eq $before_implementation
		or fail('codex created a commit; expected this script to commit the implementation');

	my ($changed_files, $changed_status) = capture_command(0, 'git', 'status', '--short', '--untracked-files=all', '--', '.');
	$changed_status == 0 or exit $changed_status;
	if ($changed_files eq '') {
		print "Implementation result:\n";
		print read_file($result_file);
		print "\n";
		fail('codex made no repository changes; see the implementation result above');
	}
	print "Changed files:\n$changed_files";

	my $commit_message = "Implement change $change_name";
	print "Commit: $commit_message\n";
	run_checked('git', 'add', '-A');
	run_checked('git', 'commit', '-m', $commit_message);
	run_checked('git', 'push', '-u', 'origin', $branch);
	ensure_clean_worktree();

	my $review_loop = File::Spec->catfile($script_dir, 'codex-review-loop.pl');
	-f $review_loop or fail("review loop not found: $review_loop");
	print "Review: ", shell_join($review_loop, $specification), "\n";
	return run_command($review_loop, $specification);
}

$SIG{INT} = sub { stop(130) };
$SIG{TERM} = sub { stop(143) };

END {
	cleanup();
}

exit main(@ARGV) unless caller;

1;
