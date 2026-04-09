defmodule Conductor.BootstrapTest do
  use ExUnit.Case, async: true

  alias Conductor.Bootstrap

  test "syncs a local spellbook source onto the sprite without uploading git metadata" do
    local_dir = Path.join(System.tmp_dir!(), "spellbook_#{System.unique_integer([:positive])}")

    File.mkdir_p!(Path.join(local_dir, "skills/demo"))
    File.mkdir_p!(Path.join(local_dir, ".git"))
    File.mkdir_p!(Path.join(local_dir, ".pytest_cache"))
    File.write!(Path.join(local_dir, "bootstrap.sh"), "echo ready\n")
    File.write!(Path.join(local_dir, "skills/demo/SKILL.md"), "demo\n")
    File.write!(Path.join(local_dir, ".git/config"), "[core]\n")
    File.write!(Path.join(local_dir, ".pytest_cache/README.md"), "cache\n")

    exec_fn = fn sprite, command, opts ->
      send(self(), {:exec, sprite, command, opts})
      {:ok, ""}
    end

    assert :ok =
             Bootstrap.ensure_spellbook("bb-weaver",
               spellbook_source: local_dir,
               exec_fn: exec_fn
             )

    assert_receive {:exec, "bb-weaver", cleanup_cmd, cleanup_opts}
    assert cleanup_cmd =~ "rm -rf '/home/sprite/spellbook'"
    assert cleanup_opts[:timeout] == 30_000

    assert_receive {:exec, "bb-weaver", "true", upload_opts}

    assert {Path.join(local_dir, "bootstrap.sh"), "/home/sprite/spellbook/bootstrap.sh"} in upload_opts[
             :files
           ]

    assert {Path.join(local_dir, "skills/demo/SKILL.md"),
            "/home/sprite/spellbook/skills/demo/SKILL.md"} in upload_opts[:files]

    refute Enum.any?(upload_opts[:files], fn {source, _destination} ->
             String.contains?(source, "/.git/") or String.contains?(source, "/.pytest_cache/")
           end)

    assert upload_opts[:timeout] == 120_000

    assert_receive {:exec, "bb-weaver", cleanup_symlinks_cmd, cleanup_symlinks_opts}
    assert cleanup_symlinks_cmd =~ "find /home/sprite/.claude/skills /home/sprite/.codex/skills"
    assert cleanup_symlinks_opts[:timeout] == 15_000

    assert_receive {:exec, "bb-weaver", bootstrap_cmd, bootstrap_opts}
    assert bootstrap_cmd == "cd /home/sprite/spellbook && bash bootstrap.sh 2>&1"
    assert bootstrap_opts[:timeout] == 60_000

    File.rm_rf!(local_dir)
  end

  test "clones generic git sources without hard-coding github transport" do
    exec_fn = fn sprite, command, opts ->
      send(self(), {:exec, sprite, command, opts})
      {:ok, ""}
    end

    source = "ssh://git.example.com/spellbook.git"

    assert :ok =
             Bootstrap.ensure_spellbook("bb-weaver", spellbook_source: source, exec_fn: exec_fn)

    assert_receive {:exec, "bb-weaver", clone_cmd, clone_opts}

    assert clone_cmd =~
             "git clone --depth 1 'ssh://git.example.com/spellbook.git' /home/sprite/spellbook"

    refute clone_cmd =~ "github.com"
    assert clone_opts[:timeout] == 60_000

    assert_receive {:exec, "bb-weaver", cleanup_symlinks_cmd, cleanup_symlinks_opts}
    assert cleanup_symlinks_cmd =~ "find /home/sprite/.claude/skills /home/sprite/.codex/skills"
    assert cleanup_symlinks_opts[:timeout] == 15_000

    assert_receive {:exec, "bb-weaver", bootstrap_cmd, bootstrap_opts}
    assert bootstrap_cmd == "cd /home/sprite/spellbook && bash bootstrap.sh 2>&1"
    assert bootstrap_opts[:timeout] == 60_000
  end

  test "returns pull failures instead of silently reusing stale remote spellbook code" do
    exec_fn = fn _sprite, command, _opts ->
      if String.contains?(command, "git pull --ff-only --quiet") do
        {:error, "pull failed", 1}
      else
        {:ok, ""}
      end
    end

    assert {:error, "spellbook clone failed: pull failed"} =
             Bootstrap.ensure_spellbook("bb-weaver",
               spellbook_source: "ssh://git.example.com/spellbook.git",
               exec_fn: exec_fn
             )
  end

  test "rejects missing local spellbook paths before attempting clone logic" do
    missing =
      Path.join(System.tmp_dir!(), "spellbook-missing-#{System.unique_integer([:positive])}")

    exec_fn = fn _sprite, _command, _opts ->
      flunk("exec_fn should not run for a missing local spellbook path")
    end

    assert {:error, reason} =
             Bootstrap.ensure_spellbook("bb-weaver",
               spellbook_source: missing,
               exec_fn: exec_fn
             )

    assert reason == "spellbook source missing: #{missing}"
  end

  test "rejects empty local spellbook directories with no uploadable files" do
    local_dir =
      Path.join(System.tmp_dir!(), "spellbook-empty-#{System.unique_integer([:positive])}")

    File.mkdir_p!(Path.join(local_dir, ".git"))

    exec_fn = fn _sprite, _command, _opts ->
      flunk("exec_fn should not run for an empty local spellbook source")
    end

    assert {:error, reason} =
             Bootstrap.ensure_spellbook("bb-weaver",
               spellbook_source: local_dir,
               exec_fn: exec_fn
             )

    assert reason == "spellbook source #{local_dir} has no files to upload"

    File.rm_rf!(local_dir)
  end
end
