#!/usr/bin/env perl
use strict;
use warnings;

use Cwd qw(abs_path);
use File::Basename qw(dirname);
use File::Spec;
use File::Temp qw(tempfile);

my $tests = 0;

sub assert_equal {
	my ($got, $want, $message) = @_;
	$tests++;
	die "$message: got [$got], want [$want]\n" if $got ne $want;
}

my $script = abs_path(File::Spec->catfile(dirname(__FILE__), 'codex-code-spec.pl'));
do $script or die "cannot load $script: " . ($@ || $!);

assert_equal(activity_frame(0), "\xC2\xB7", 'animation starts at the small central point');
assert_equal(activity_frame(0.25), "\xE2\x80\xA2", 'animation advances after 250 ms');
assert_equal(activity_frame(0.50), "\xE2\x97\x8F", 'animation reaches the large central point');
assert_equal(activity_frame(0.75), "\xE2\x80\xA2", 'animation contracts after 750 ms');
assert_equal(activity_frame(1), "\xC2\xB7", 'animation repeats after one second');

my $rendered = '';
open my $progress_output, '>:raw', \$rendered or die "cannot create progress capture: $!";
my ($log, $output_log) = tempfile();
close $log;
my $progress = new_progress_state('Implement', 1, $progress_output);
my $status = monitor_command(
	$progress,
	$output_log,
	$^X,
	'-e',
	'select undef, undef, undef, 0.85',
);
close $progress_output;
unlink $output_log;

assert_equal($status, 0, 'silent child exits successfully');
my %rendered_frames;
for my $render (split /\r/, $rendered) {
	for my $frame ("\xC2\xB7", "\xE2\x80\xA2", "\xE2\x97\x8F") {
		$rendered_frames{$frame} = 1 if $render =~ /\Q$frame\E\z/;
	}
}
assert_equal(scalar keys %rendered_frames, 3, 'silent child renders distinct animation frames at timed intervals');

print "codex-code-spec unit tests passed ($tests assertions)\n";
