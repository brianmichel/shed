defmodule Garden.LocalSandbox.Guardrails do
  @moduledoc false

  defdelegate implementation(), to: Garden.Guardrails
  defdelegate allow_command(sandbox, spec), to: Garden.Guardrails
  defdelegate allow_file_action(sandbox, action, path, attrs \\ %{}), to: Garden.Guardrails
  defdelegate normalize_cwd(sandbox, cwd), to: Garden.Guardrails
  defdelegate sanitize_env(sandbox, env), to: Garden.Guardrails
  defdelegate allow_signal(sandbox, signal), to: Garden.Guardrails
end
