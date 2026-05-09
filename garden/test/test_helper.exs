ExUnit.start()

if Application.get_env(:garden, :start_repo, true) do
  Ecto.Adapters.SQL.Sandbox.mode(Garden.Repo, :manual)
end
