defmodule Garden.LocalSandbox.Guardrails.MacOS do
  @moduledoc false

  defdelegate allow_command(sandbox, spec), to: Garden.Guardrails.MacOS
  defdelegate allow_file_action(sandbox, action, path, attrs), to: Garden.Guardrails.MacOS
  defdelegate normalize_cwd(sandbox, cwd), to: Garden.Guardrails.MacOS
  defdelegate sanitize_env(sandbox, env), to: Garden.Guardrails.MacOS
  defdelegate allow_signal(sandbox, signal), to: Garden.Guardrails.MacOS
end
