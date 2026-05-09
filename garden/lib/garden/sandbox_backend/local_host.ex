defmodule Garden.SandboxBackend.LocalHost do
  @behaviour Garden.SandboxBackend

  alias Garden.Guardrails
  alias Garden.Sandboxes.LocalHostRuntime

  @root Path.expand("../tmp/garden/sandboxes", File.cwd!())

  @impl true
  def name, do: "local_host"

  @impl true
  def setup_sandbox(sandbox, store) do
    root = workspace_root(sandbox)
    File.mkdir_p!(root)
    seed_files(root, sandbox.id)
    LocalHostRuntime.ensure_started(sandbox.id, store, root, sandbox)
  end

  @impl true
  def teardown_sandbox(sandbox_id), do: LocalHostRuntime.stop(sandbox_id)
  @impl true
  def start_command(sandbox_id, command), do: LocalHostRuntime.start_command(sandbox_id, command)
  @impl true
  def send_stdin(sandbox_id, command_id, data), do: LocalHostRuntime.send_stdin(sandbox_id, command_id, data)
  @impl true
  def cancel_command(sandbox_id, command_id, attrs), do: LocalHostRuntime.cancel_command(sandbox_id, command_id, attrs)
  @impl true
  def kill_command(sandbox_id, command_id), do: LocalHostRuntime.kill_command(sandbox_id, command_id)

  @impl true
  def list_files(sandbox, path) do
    with :ok <- Guardrails.allow_file_action(enriched(sandbox), :list, path, %{}),
         {:ok, resolved} <- resolve_path(sandbox, path),
         true <- File.dir?(resolved) || {:error, :file_not_found} do
      entries = resolved |> File.ls!() |> Enum.sort()
      {:ok, %{path: normalized_display_path(sandbox, resolved), entries: entries}}
    else
      {:error, _} = err -> err
      false -> {:error, :file_not_found}
    end
  end

  @impl true
  def read_file(sandbox, path) do
    with :ok <- Guardrails.allow_file_action(enriched(sandbox), :read, path, %{}),
         {:ok, resolved} <- resolve_path(sandbox, path),
         true <- File.exists?(resolved) || {:error, :file_not_found},
         {:ok, content} <- File.read(resolved) do
      {:ok, %{path: normalized_display_path(sandbox, resolved), content: content}}
    else
      {:error, _} = err -> err
      false -> {:error, :file_not_found}
    end
  end

  @impl true
  def write_file(sandbox, path, content) do
    with :ok <- Guardrails.allow_file_action(enriched(sandbox), :write, path, %{bytes: byte_size(content)}),
         {:ok, resolved} <- resolve_path(sandbox, path) do
      resolved |> Path.dirname() |> File.mkdir_p!()
      File.write!(resolved, content)
      {:ok, %{path: normalized_display_path(sandbox, resolved), bytes: byte_size(content)}}
    end
  end

  @impl true
  def workspace_root(sandbox), do: Path.join([@root, sandbox.id || sandbox[:id], "workspace"])

  defp resolve_path(sandbox, path) do
    root = workspace_root(sandbox)
    display_root = "/workspace"
    path = if path in [nil, ""], do: display_root, else: path
    rel = String.replace_prefix(path, display_root, "") |> String.trim_leading("/")
    resolved = Path.expand(rel, root)
    if String.starts_with?(resolved, root), do: {:ok, resolved}, else: {:error, :path_outside_workspace}
  end

  defp normalized_display_path(sandbox, resolved) do
    root = workspace_root(sandbox)
    suffix = String.replace_prefix(resolved, root, "")
    "/workspace" <> if(suffix == "", do: "", else: suffix)
  end

  defp seed_files(root, sandbox_id) do
    readme = Path.join(root, "README.txt")
    notes = Path.join(root, "notes.txt")
    unless File.exists?(readme), do: File.write!(readme, "Sandbox #{sandbox_id}\n")
    unless File.exists?(notes), do: File.write!(notes, "hello from garden\n")
  end

  defp enriched(sandbox), do: Map.put(sandbox, :workspace_root, workspace_root(sandbox))
end
