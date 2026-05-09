defmodule Garden.SeedSessions do
  @moduledoc """
  In-memory session directory and message dispatcher for Seed connections.
  """

  alias Garden.SeedSessions.Store

  def issue_session(sandbox_id), do: Store.issue_session(sandbox_id)
  def list_sessions, do: Store.list_sessions()
  def find_by_sandbox(sandbox_id), do: Store.find_by_sandbox(sandbox_id)
  def authenticate(session_key, sandbox_id), do: Store.authenticate(session_key, sandbox_id)
  def connect(session_id, pid), do: Store.connect(session_id, pid)
  def disconnect(session_id), do: Store.disconnect(session_id)
  def record_inbound(session_id, message), do: Store.record_inbound(session_id, message)
  def dispatch(session_id, type, payload, opts \\ []), do: Store.dispatch(session_id, type, payload, opts)
  def get(session_id), do: Store.get(session_id)
  def topic(session_id), do: "seed_sessions:" <> session_id
  def index_topic, do: "seed_sessions:index"
end
