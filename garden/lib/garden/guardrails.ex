defmodule Garden.Guardrails do
  @moduledoc """
  Environment-agnostic guardrail behavior for sandbox execution policy.

  This behavior is intended to be shared across local, container, VM, and other
  backends. Specific runtimes can invoke it before command, file, process, or
  other sandbox actions.
  """

  @type sandbox :: map()
  @type command_spec :: %{
          required(:command) => String.t(),
          optional(:cwd) => String.t(),
          optional(:env) => map(),
          optional(:metadata) => map()
        }

  @callback allow_command(sandbox, command_spec) :: :ok | {:error, term()}
  @callback allow_file_action(sandbox, action :: atom(), path :: String.t(), attrs :: map()) ::
              :ok | {:error, term()}
  @callback normalize_cwd(sandbox, cwd :: String.t() | nil) :: {:ok, String.t()} | {:error, term()}
  @callback sanitize_env(sandbox, env :: map()) :: map()
  @callback allow_signal(sandbox, signal :: String.t()) :: :ok | {:error, term()}

  def implementation do
    Application.get_env(:garden, :guardrails, Garden.Guardrails.Default)
  end

  def allow_command(sandbox, spec), do: implementation().allow_command(sandbox, spec)
  def allow_file_action(sandbox, action, path, attrs \\ %{}), do: implementation().allow_file_action(sandbox, action, path, attrs)
  def normalize_cwd(sandbox, cwd), do: implementation().normalize_cwd(sandbox, cwd)
  def sanitize_env(sandbox, env), do: implementation().sanitize_env(sandbox, env)
  def allow_signal(sandbox, signal), do: implementation().allow_signal(sandbox, signal)
end
