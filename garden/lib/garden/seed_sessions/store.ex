defmodule Garden.SeedSessions.Store do
  @moduledoc false

  use GenServer

  alias Garden.Persistence
  alias Garden.SeedProtocol.Message

  def start_link(arg \\ []) do
    GenServer.start_link(__MODULE__, arg, name: __MODULE__)
  end

  def issue_session(sandbox_id), do: GenServer.call(__MODULE__, {:issue_session, sandbox_id})
  def list_sessions, do: GenServer.call(__MODULE__, :list_sessions)
  def find_by_sandbox(sandbox_id), do: GenServer.call(__MODULE__, {:find_by_sandbox, sandbox_id})
  def authenticate(session_key, sandbox_id), do: GenServer.call(__MODULE__, {:authenticate, session_key, sandbox_id})
  def connect(session_id, pid), do: GenServer.call(__MODULE__, {:connect, session_id, pid})
  def disconnect(session_id), do: GenServer.call(__MODULE__, {:disconnect, session_id})
  def record_inbound(session_id, message), do: GenServer.call(__MODULE__, {:record_inbound, session_id, message})
  def dispatch(session_id, type, payload, opts \\ []), do: GenServer.call(__MODULE__, {:dispatch, session_id, type, payload, opts})
  def get(session_id), do: GenServer.call(__MODULE__, {:get, session_id})
  def reset!, do: GenServer.call(__MODULE__, :reset)

  @impl true
  def init(_arg) do
    {:ok, %{sessions: %{}, key_index: %{}, dedupe: %{}}}
  end

  @impl true
  def handle_call(:reset, _from, _state) do
    {:reply, :ok, %{sessions: %{}, key_index: %{}, dedupe: %{}}}
  end

  def handle_call({:find_by_sandbox, sandbox_id}, _from, state) do
    status_rank = fn
      :connected -> 2
      :issued -> 1
      _ -> 0
    end

    session =
      state.sessions
      |> Map.values()
      |> Enum.filter(&(&1.sandbox_id == sandbox_id))
      |> Enum.sort_by(&{status_rank.(&1.status), &1.updated_at}, :desc)
      |> List.first()

    {:reply, if(session, do: {:ok, session}, else: {:error, :session_not_found}), state}
  end

  def handle_call(:list_sessions, _from, state) do
    sessions =
      state.sessions
      |> Map.values()
      |> Enum.sort_by(& &1.inserted_at, {:desc, DateTime})

    {:reply, sessions, state}
  end

  def handle_call({:issue_session, sandbox_id}, _from, state) do
    session = %{
      session_id: id("sess"),
      session_key: id("seedkey"),
      sandbox_id: sandbox_id,
      status: :issued,
      socket_pid: nil,
      last_seed_seq_seen: 0,
      last_garden_seq_sent: 0,
      last_garden_seq_acked: 0,
      sent_messages: %{},
      messages: [],
      inserted_at: now(),
      updated_at: now()
    }

    state = put_session(state, session)
    state = put_in(state, [:key_index, session.session_key], session.session_id)
    broadcast_session_update(session)
    {:reply, {:ok, session}, state}
  end

  def handle_call({:authenticate, session_key, sandbox_id}, _from, state) do
    with {:ok, session_id} <- fetch_key(state, session_key),
         {:ok, session} <- fetch_session(state, session_id),
         true <- session.sandbox_id == sandbox_id || {:error, :sandbox_mismatch} do
      {:reply, {:ok, session}, state}
    else
      error -> {:reply, error, state}
    end
  end

  def handle_call({:connect, session_id, pid}, _from, state) do
    with {:ok, session} <- fetch_session(state, session_id) do
      Process.monitor(pid)
      updated = %{session | socket_pid: pid, status: :connected, updated_at: now()}
      state = put_session(state, updated)
      broadcast_session_update(updated)
      {:reply, {:ok, updated}, state}
    else
      error -> {:reply, error, state}
    end
  end

  def handle_call({:disconnect, session_id}, _from, state) do
    with {:ok, session} <- fetch_session(state, session_id) do
      updated = %{session | socket_pid: nil, status: :disconnected, updated_at: now()}
      state = put_session(state, updated)
      broadcast_session_update(updated)
      {:reply, :ok, state}
    else
      error -> {:reply, error, state}
    end
  end

  def handle_call({:record_inbound, session_id, %Message{} = message}, _from, state) do
    with {:ok, session} <- fetch_session(state, session_id),
         :ok <- ensure_monotonic(session, message),
         false <- duplicate?(state, message.message_id) do
      session =
        session
        |> Map.put(:last_seed_seq_seen, message.seq)
        |> Map.put(:updated_at, now())
        |> append_message(%{direction: "inbound", message: Message.to_map(message)})

      state = put_session(state, session)
      state = remember_message(state, message.message_id)
      broadcast_session_update(session)
      mapped = Message.to_map(message)
      Phoenix.PubSub.broadcast(Garden.PubSub, session_topic(session_id), {:seed_message, mapped})
      Garden.Sandboxes.Store.protocol_message(session_id, mapped)
      {:reply, {:ok, session}, state, {:continue, {:handle_inbound, session_id, message.type, message}}}
    else
      true -> {:reply, {:error, :duplicate_message}, state}
      error -> {:reply, error, state}
    end
  end

  def handle_call({:dispatch, session_id, type, payload, opts}, _from, state) do
    with {:ok, _} <- fetch_session(state, session_id) do
      {state, message} = do_dispatch(state, session_id, type, payload, opts)
      {:reply, {:ok, message}, state}
    else
      error -> {:reply, error, state}
    end
  end

  def handle_call({:get, session_id}, _from, state) do
    {:reply, fetch_session(state, session_id), state}
  end

  @impl true
  def handle_continue({:handle_inbound, session_id, "seed.hello", _message}, state) do
    case fetch_session(state, session_id) do
      {:ok, session} ->
        {state, _} =
          do_dispatch(state, session_id, "garden.hello", %{
            "protocol_version" => "1",
            "session_id" => session_id,
            "sandbox_id" => session.sandbox_id,
            "heartbeat_interval_ms" => 10_000,
            "ack_timeout_ms" => 5_000
          }, expects_ack: false)

        {:noreply, state}

      _ ->
        {:noreply, state}
    end
  end

  def handle_continue({:handle_inbound, session_id, "seed.register", _message}, state) do
    case fetch_session(state, session_id) do
      {:ok, session} ->
        lease_expires_at =
          case Garden.Sandboxes.get_sandbox(session.sandbox_id) do
            {:ok, sandbox} -> sandbox.lease["expires_at"]
            _ -> nil
          end

        payload =
          %{
            "session_id" => session_id,
            "sandbox_id" => session.sandbox_id,
            "max_session_duration_ms" => 7_200_000
          }
          |> then(&if(lease_expires_at, do: Map.put(&1, "lease_expires_at", lease_expires_at), else: &1))

        {state, _} = do_dispatch(state, session_id, "garden.registered", payload, expects_ack: false)
        {:noreply, state}

      _ ->
        {:noreply, state}
    end
  end

  def handle_continue({:handle_inbound, session_id, "seed.heartbeat", _message}, state) do
    case fetch_session(state, session_id) do
      {:ok, session} ->
        lease_expires_at =
          case Garden.Sandboxes.get_sandbox(session.sandbox_id) do
            {:ok, sandbox} -> sandbox.lease["expires_at"]
            _ -> nil
          end

        payload =
          %{
            "server_time" => DateTime.to_iso8601(DateTime.utc_now()),
            "status" => "ok"
          }
          |> then(&if(lease_expires_at, do: Map.put(&1, "lease_expires_at", lease_expires_at), else: &1))

        {state, _} = do_dispatch(state, session_id, "garden.heartbeat_ack", payload, expects_ack: false)
        {:noreply, state}

      _ ->
        {:noreply, state}
    end
  end

  def handle_continue({:handle_inbound, session_id, "seed.resume", message}, state) do
    case fetch_session(state, session_id) do
      {:ok, session} ->
        last_garden_seq_seen = message.payload["last_garden_seq_seen"]

        session.sent_messages
        |> Map.values()
        |> Enum.filter(&(&1.seq > last_garden_seq_seen))
        |> Enum.sort_by(& &1.seq)
        |> Enum.each(fn msg ->
          Phoenix.PubSub.broadcast(Garden.PubSub, session_topic(session_id), {:garden_message, Message.to_map(msg)})
        end)

        {state, _} =
          do_dispatch(state, session_id, "garden.resume", %{
            "status" => "ok",
            "session_id" => session_id,
            "replay_from_garden_seq" => last_garden_seq_seen
          }, expects_ack: false)

        {:noreply, state}

      _ ->
        {:noreply, state}
    end
  end

  def handle_continue({:handle_inbound, _session_id, _type, _message}, state) do
    {:noreply, state}
  end

  @impl true
  def handle_info({:DOWN, _ref, :process, pid, _reason}, state) do
    sessions =
      Enum.reduce(state.sessions, state.sessions, fn {session_id, session}, acc ->
        if session.socket_pid == pid do
          updated = %{session | socket_pid: nil, status: :disconnected, updated_at: now()}
          broadcast_session_update(updated)
          Map.put(acc, session_id, updated)
        else
          acc
        end
      end)

    {:noreply, %{state | sessions: sessions}}
  end

  defp do_dispatch(state, session_id, type, payload, opts) do
    {:ok, session} = fetch_session(state, session_id)
    seq = session.last_garden_seq_sent + 1

    message = %Message{
      version: "1",
      type: type,
      message_id: id("msg"),
      ack_id: Keyword.get(opts, :ack_id),
      request_id: Keyword.get(opts, :request_id, id("req")),
      session_id: session.session_id,
      sandbox_id: session.sandbox_id,
      seq: seq,
      timestamp: DateTime.to_iso8601(now()),
      expects_ack: Keyword.get(opts, :expects_ack, true),
      reply_to: Keyword.get(opts, :reply_to),
      payload: payload
    }

    updated =
      session
      |> Map.put(:last_garden_seq_sent, seq)
      |> Map.put(:sent_messages, Map.put(session.sent_messages, message.message_id, message))
      |> Map.put(:updated_at, now())
      |> append_message(%{direction: "outbound", message: Message.to_map(message)})

    state = put_session(state, updated)
    broadcast_session_update(updated)
    Phoenix.PubSub.broadcast(Garden.PubSub, session_topic(session_id), {:garden_message, Message.to_map(message)})
    {state, message}
  end

  defp fetch_key(state, session_key) do
    case Map.fetch(state.key_index, session_key) do
      {:ok, session_id} -> {:ok, session_id}
      :error -> {:error, :invalid_session_key}
    end
  end

  defp fetch_session(state, session_id) do
    case Map.fetch(state.sessions, session_id) do
      {:ok, session} -> {:ok, session}
      :error -> {:error, :session_not_found}
    end
  end

  defp ensure_monotonic(session, message) do
    if message.seq > session.last_seed_seq_seen do
      :ok
    else
      {:error, :out_of_order_message}
    end
  end

  defp duplicate?(state, message_id), do: Map.has_key?(state.dedupe, message_id)
  defp remember_message(state, message_id), do: put_in(state, [:dedupe, message_id], true)

  defp put_session(state, session) do
    Persistence.persist_session(session)
    put_in(state, [:sessions, session.session_id], session)
  end
  defp append_message(session, entry) do
    messages = (session.messages ++ [entry]) |> Enum.take(-100)
    %{session | messages: messages}
  end

  defp broadcast_session_update(session) do
    Phoenix.PubSub.broadcast(Garden.PubSub, Garden.SeedSessions.index_topic(), {:session_updated, session})
    Phoenix.PubSub.broadcast(Garden.PubSub, session_topic(session.session_id), {:session_updated, session})
  end

  defp session_topic(session_id), do: Garden.SeedSessions.topic(session_id)
  defp id(prefix), do: prefix <> "_" <> Base.encode16(:crypto.strong_rand_bytes(6), case: :lower)
  defp now, do: DateTime.utc_now() |> DateTime.truncate(:second)
end
