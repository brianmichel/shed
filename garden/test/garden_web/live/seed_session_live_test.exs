defmodule GardenWeb.SeedSessionLiveTest do
  use GardenWeb.ConnCase, async: false

  alias Garden.SeedSessions
  alias Garden.SeedSessions.Store

  import Phoenix.LiveViewTest

  setup do
    Store.reset!()
    :ok
  end

  test "lists issued sessions", %{conn: conn} do
    {:ok, _session} = SeedSessions.issue_session("sbx_live")

    {:ok, _view, html} = live(conn, ~p"/seed/sessions")

    assert html =~ "Seed sessions"
    assert html =~ "sbx_live"
  end
end
