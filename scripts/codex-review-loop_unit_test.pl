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

my $script = abs_path(File::Spec->catfile(dirname(__FILE__), 'codex-review-loop.pl'));
do $script or die "cannot load $script: " . ($@ || $!);

my $context_output = '';
open my $context_handle, '>:raw', \$context_output or die "cannot create context capture: $!";
{
	local $ENV{NO_COLOR};
	delete $ENV{NO_COLOR};
	print_initial_context({
		branch             => 'change/004-decoder-service',
		findings_file      => '/tmp/apih-review.test/findings.md',
		repo_root          => '/home/vito/go/src/apihydra',
		review_base        => 'origin/master',
		review_base_commit => '47d4f35c2759e81216efb909c66d7e58ba43e503',
		review_options     => '--base 47d4f35c2759e81216efb909c66d7e58ba43e503',
		specification      => 'agent/specs/004-decoder-service.md',
	}, 1, $context_handle);
}
close $context_handle;
assert_equal(
	$context_output,
	"\n\033[37mRepository:\033[0m \033[34m/home/vito/go/src/apihydra\033[0m\n"
		. "\033[37mSpecification:\033[0m \033[35magent/specs/004-decoder-service.md\033[0m\n"
		. "\033[37mBranch:\033[0m \033[32mchange/004-decoder-service\033[0m\n"
		. "\033[37mBase:\033[0m \033[33morigin/master\033[0m\n"
		. "\033[37mPinned base:\033[0m \033[33m47d4f35c2759e81216efb909c66d7e58ba43e503\033[0m\n"
		. "\033[37mReview options:\033[0m \033[35m--base 47d4f35c2759e81216efb909c66d7e58ba43e503\033[0m\n"
		. "\033[37mFindings:\033[0m \033[34m/tmp/apih-review.test/findings.md\033[0m\n",
	'review context extends the implementation order with consistent colors',
);

my $command_output = '';
open my $command_handle, '>:raw', \$command_output or die "cannot create command capture: $!";
{
	local *STDOUT = $command_handle;
	print_rendered_command("codex exec --json\n< findings.md");
}
close $command_handle;
my $command_separator = '-' x length('codex exec --json');
assert_equal(
	$command_output,
	"$command_separator\ncodex exec --json\n< findings.md\n$command_separator\n",
	'multiline review commands use the longest line without extending the separator',
);

assert_equal(
	join("\0", parse_review_options('--model', 'gpt-5', '--title', 'Review title', '--base', 'develop')),
	join("\0", 'develop', '--model', 'gpt-5', '--title', 'Review title'),
	'option values remain distinct from positional review prompts',
);

my $rendered = '';
open my $progress_output, '>:raw', \$rendered or die "cannot create progress capture: $!";
my ($log, $output_log) = tempfile();
close $log;
my $progress = APIHydra::Progress->new(terminal => 1, output => $progress_output);
my $status = monitor_command(
	$progress,
	undef,
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

print "codex-review-loop unit tests passed ($tests assertions)\n";
