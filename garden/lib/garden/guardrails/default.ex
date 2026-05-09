defmodule Garden.Guardrails.Default do
  @moduledoc """
  Default permissive-but-workspace-bounded guardrails.
  """

  @behaviour Garden.Guardrails

  @blocked_command_prefixes ["sudo", "docker", "podman", "osascript", "open"]
  @blocked_signals ["SIGSTOP"]

  @impl true
  def allow_command(sandbox, spec) do
    command = String.trim(Map.get(spec, :command, ""))

    cond do
      command == "" -> {:error, :empty_command}
      Enum.any?(@blocked_command_prefixes, &String.starts_with?(command, &1)) -> {:error, {:blocked_command, command}}
      true ->
        with :ok <- within_workspace(sandbox, Map.get(spec, :cwd)),
             :ok <- check_env(spec) do
          :ok
        end
    end
  end

  @impl true
  def allow_file_action(sandbox, _action, path, _attrs), do: within_workspace(sandbox, path)

  @impl true
  def normalize_cwd(sandbox, nil), do: {:ok, workspace_root(sandbox)}

  def normalize_cwd(sandbox, cwd) when is_binary(cwd) do
    expanded = if Path.type(cwd) == :absolute, do: Path.expand(cwd), else: Path.expand(cwd, workspace_root(sandbox))
    if String.starts_with?(expanded, workspace_root(sandbox)), do: {:ok, expanded}, else: {:error, :cwd_outside_workspace}
  end

  @impl true
  def sanitize_env(_sandbox, env) do
    env
    |> Enum.reject(fn {key, _value} -> String.starts_with?(to_string(key), "AWS_") end)
    |> Enum.into(%{})
  end

  @impl true
  def allow_signal(_sandbox, signal) when signal in @blocked_signals, do: {:error, {:blocked_signal, signal}}
  def allow_signal(_sandbox, _signal), do: :ok

  defp check_env(spec) do
    env = Map.get(spec, :env, %{})
    if is_map(env), do: :ok, else: {:error, :invalid_env}
  end

  defp within_workspace(_sandbox, nil), do: :ok
  defp within_workspace(sandbox, path) do
    root = workspace_root(sandbox)
    expanded = if Path.type(path) == :absolute, do: Path.expand(path), else: Path.expand(path, root)
    if String.starts_with?(expanded, root), do: :ok, else: {:error, :path_outside_workspace}
  end

  defp workspace_root(sandbox) do
    Map.get(sandbox, :workspace_root) || Map.get(sandbox, "workspace_root") || "/workspace"
  end
end
