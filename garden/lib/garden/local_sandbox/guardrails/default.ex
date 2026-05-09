defmodule Garden.LocalSandbox.Guardrails.Default do
  @moduledoc false

  defdelegate allow_command(sandbox, spec), to: Garden.Guardrails.Default
  defdelegate allow_file_action(sandbox, action, path, attrs), to: Garden.Guardrails.Default
  defdelegate normalize_cwd(sandbox, cwd), to: Garden.Guardrails.Default
  defdelegate sanitize_env(sandbox, env), to: Garden.Guardrails.Default
  defdelegate allow_signal(sandbox, signal), to: Garden.Guardrails.Default
end
