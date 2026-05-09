defmodule Garden.GuardrailsTest do
  use ExUnit.Case, async: true

  alias Garden.Guardrails

  @sandbox %{workspace_root: "/workspace"}

  test "default implementation is configured" do
    assert Guardrails.implementation() == Garden.Guardrails.Default
  end

  test "default guardrails allow workspace command" do
    assert :ok = Guardrails.allow_command(@sandbox, %{command: "ls -la", cwd: "/workspace", env: %{}})
  end

  test "default guardrails reject cwd outside workspace" do
    assert {:error, :path_outside_workspace} = Guardrails.allow_file_action(@sandbox, :read, "/tmp/secret", %{})
  end

  test "default guardrails block dangerous prefixes" do
    assert {:error, {:blocked_command, "sudo rm -rf /"}} =
             Guardrails.allow_command(@sandbox, %{command: "sudo rm -rf /", cwd: "/workspace", env: %{}})
  end
end
