defmodule Garden.Guardrails.MacOS do
  @moduledoc """
  macOS-focused guardrails layered on top of the default policy.
  """

  @behaviour Garden.Guardrails

  @impl true
  def allow_command(sandbox, spec) do
    with :ok <- Garden.Guardrails.Default.allow_command(sandbox, spec) do
      command = String.trim(Map.get(spec, :command, ""))
      if String.contains?(command, "/System") or String.starts_with?(command, "open "), do: {:error, :macos_protected_command}, else: :ok
    end
  end

  @impl true
  def allow_file_action(sandbox, action, path, attrs), do: Garden.Guardrails.Default.allow_file_action(sandbox, action, path, attrs)
  @impl true
  def normalize_cwd(sandbox, cwd), do: Garden.Guardrails.Default.normalize_cwd(sandbox, cwd)
  @impl true
  def sanitize_env(sandbox, env), do: Garden.Guardrails.Default.sanitize_env(sandbox, env)
  @impl true
  def allow_signal(sandbox, signal), do: Garden.Guardrails.Default.allow_signal(sandbox, signal)
end
