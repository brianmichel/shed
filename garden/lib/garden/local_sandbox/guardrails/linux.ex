defmodule Garden.LocalSandbox.Guardrails.Linux do
  @moduledoc false

  defdelegate allow_command(sandbox, spec), to: Garden.Guardrails.Linux
  defdelegate allow_file_action(sandbox, action, path, attrs), to: Garden.Guardrails.Linux
  defdelegate normalize_cwd(sandbox, cwd), to: Garden.Guardrails.Linux
  defdelegate sanitize_env(sandbox, env), to: Garden.Guardrails.Linux
  defdelegate allow_signal(sandbox, signal), to: Garden.Guardrails.Linux
end
