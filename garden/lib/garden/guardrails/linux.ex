defmodule Garden.Guardrails.Linux do
  @moduledoc """
  Linux-focused guardrails layered on top of the default policy.
  """

  @behaviour Garden.Guardrails

  @impl true
  def allow_command(sandbox, spec) do
    with :ok <- Garden.Guardrails.Default.allow_command(sandbox, spec) do
      command = String.trim(Map.get(spec, :command, ""))
      if String.contains?(command, "/proc") or String.contains?(command, "/sys"), do: {:error, :linux_protected_path}, else: :ok
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
