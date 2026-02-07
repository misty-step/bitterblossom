package cobra

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Command is a minimal subset of spf13/cobra.Command used by bb.
type Command struct {
	Use           string
	Short         string
	RunE          func(cmd *Command, args []string) error
	SilenceUsage  bool
	SilenceErrors bool

	parent   *Command
	children []*Command
	flags    *flag.FlagSet
	out      io.Writer
	err      io.Writer
	args     []string
	ctx      context.Context
}

// AddCommand registers child subcommands.
func (c *Command) AddCommand(children ...*Command) {
	for _, child := range children {
		if child == nil {
			continue
		}
		child.parent = c
		c.children = append(c.children, child)
	}
}

// Flags returns the command-specific flag set.
func (c *Command) Flags() *FlagSet {
	if c.flags == nil {
		c.flags = flag.NewFlagSet(c.commandName(), flag.ContinueOnError)
		c.flags.SetOutput(c.ErrOrStderr())
	}
	return &FlagSet{set: c.flags}
}

// Execute runs command parsing and dispatch from os.Args.
func (c *Command) Execute() error {
	return c.ExecuteContext(context.Background())
}

// ExecuteContext runs command parsing and dispatch from os.Args with context.
func (c *Command) ExecuteContext(ctx context.Context) error {
	args := c.args
	if args == nil {
		args = os.Args[1:]
	}
	return c.execute(ctx, args)
}

func (c *Command) execute(ctx context.Context, args []string) error {
	c.ctx = ctx

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		if child := c.findChild(args[0]); child != nil {
			return child.execute(ctx, args[1:])
		}
	}

	flagSet := c.Flags().set
	if err := flagSet.Parse(args); err != nil {
		return err
	}
	remaining := flagSet.Args()

	if len(remaining) > 0 && !strings.HasPrefix(remaining[0], "-") {
		if child := c.findChild(remaining[0]); child != nil {
			return child.execute(ctx, remaining[1:])
		}
		if c.RunE == nil && len(c.children) > 0 {
			return fmt.Errorf("unknown command %q for %q", remaining[0], c.commandName())
		}
	}

	if c.RunE != nil {
		return c.RunE(c, remaining)
	}
	if len(c.children) > 0 {
		return fmt.Errorf("no command specified")
	}
	return nil
}

func (c *Command) findChild(name string) *Command {
	for _, child := range c.children {
		if child.commandName() == name {
			return child
		}
	}
	return nil
}

func (c *Command) commandName() string {
	fields := strings.Fields(c.Use)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// SetOut sets command stdout.
func (c *Command) SetOut(out io.Writer) {
	c.out = out
}

// SetErr sets command stderr.
func (c *Command) SetErr(err io.Writer) {
	c.err = err
}

// OutOrStdout resolves command stdout.
func (c *Command) OutOrStdout() io.Writer {
	if c.out != nil {
		return c.out
	}
	if c.parent != nil {
		return c.parent.OutOrStdout()
	}
	return os.Stdout
}

// ErrOrStderr resolves command stderr.
func (c *Command) ErrOrStderr() io.Writer {
	if c.err != nil {
		return c.err
	}
	if c.parent != nil {
		return c.parent.ErrOrStderr()
	}
	return os.Stderr
}

// SetArgs overrides os.Args parsing for tests.
func (c *Command) SetArgs(args []string) {
	c.args = make([]string, len(args))
	copy(c.args, args)
}

// Context returns the execution context.
func (c *Command) Context() context.Context {
	if c.ctx != nil {
		return c.ctx
	}
	if c.parent != nil {
		return c.parent.Context()
	}
	return context.Background()
}

// FlagSet wraps stdlib flags with cobra-compatible helpers.
type FlagSet struct {
	set *flag.FlagSet
}

func (f *FlagSet) StringVar(p *string, name, value, usage string) {
	f.set.StringVar(p, name, value, usage)
}

func (f *FlagSet) IntVar(p *int, name string, value int, usage string) {
	f.set.IntVar(p, name, value, usage)
}

func (f *FlagSet) DurationVar(p *time.Duration, name string, value time.Duration, usage string) {
	f.set.DurationVar(p, name, value, usage)
}

func (f *FlagSet) BoolVar(p *bool, name string, value bool, usage string) {
	f.set.BoolVar(p, name, value, usage)
}
