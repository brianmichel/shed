defmodule Garden.Persistence do
  @moduledoc false

  alias Garden.Repo
  alias Garden.Persistence.{CommandRecord, CommandEventRecord, SandboxEventRecord, SandboxRecord, SeedSessionRecord}

  def persist_sandbox(sandbox) do
    upsert(SandboxRecord, sandbox.id, %{
      sandbox_id: sandbox.id,
      state: sandbox.state,
      environment: sandbox.environment,
      template: sandbox.template,
      metadata: sandbox.metadata,
      capabilities: sandbox.capabilities,
      lease: sandbox.lease,
      snapshot: sandbox
    })
  end

  def persist_command(command) do
    upsert(CommandRecord, command.id, %{
      command_id: command.id,
      sandbox_id: command.sandbox_id,
      state: command.state,
      command: command.command,
      cwd: command.cwd,
      env: command.env,
      stdin: command.stdin,
      timeout_ms: command.timeout_ms,
      metadata: command.metadata,
      pid: command.pid,
      exit_code: command.exit_code,
      signal: command.signal,
      started_at: command.started_at,
      completed_at: command.completed_at,
      snapshot: command
    })
  end

  def persist_sandbox_event(event) do
    insert(SandboxEventRecord, %{
      event_id: event.id,
      sandbox_id: event.sandbox_id,
      seq: event.seq,
      type: event.type,
      timestamp: event.timestamp,
      data: event.data
    })
  end

  def persist_command_event(event) do
    insert(CommandEventRecord, %{
      event_id: event.id,
      sandbox_id: event.sandbox_id,
      command_id: event.command_id,
      seq: event.seq,
      type: event.type,
      timestamp: event.timestamp,
      data: event.data
    })
  end

  def persist_session(session) do
    upsert(SeedSessionRecord, session.session_id, %{
      session_id: session.session_id,
      session_key: session.session_key,
      sandbox_id: session.sandbox_id,
      status: to_string(session.status),
      last_seed_seq_seen: session.last_seed_seq_seen,
      last_garden_seq_sent: session.last_garden_seq_sent,
      last_garden_seq_acked: session.last_garden_seq_acked,
      messages: session.messages,
      snapshot: %{
        session_id: session.session_id,
        sandbox_id: session.sandbox_id,
        status: to_string(session.status),
        last_seed_seq_seen: session.last_seed_seq_seen,
        last_garden_seq_sent: session.last_garden_seq_sent,
        last_garden_seq_acked: session.last_garden_seq_acked,
        socket_pid: if(session.socket_pid, do: inspect(session.socket_pid), else: nil)
      }
    })
  end

  defp upsert(schema, key, attrs) do
    with_repo(fn ->
      changeset = schema.changeset(struct(schema), attrs)
      Repo.insert(changeset, on_conflict: {:replace_all_except, [:id, :inserted_at]}, conflict_target: schema.conflict_target(key))
    end)
  end

  defp insert(schema, attrs) do
    with_repo(fn ->
      schema.changeset(struct(schema), attrs) |> Repo.insert(on_conflict: :nothing)
    end)
  end

  defp with_repo(fun) do
    if Application.get_env(:garden, :start_repo, true) do
      fun.()
    else
      :ok
    end
  rescue
    _ -> :ok
  end
end
