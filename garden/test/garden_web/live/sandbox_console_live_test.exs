defmodule GardenWeb.SandboxConsoleLiveTest do
  use GardenWeb.ConnCase, async: false

  import Phoenix.LiveViewTest

  alias Garden.Sandboxes
  alias Garden.Sandboxes.Store

  setup do
    Store.reset!()
    {:ok, _sandbox} = Sandboxes.ensure_sandbox("sbx_console_test")
    :ok
  end

  test "shows command output after running a command", %{conn: conn} do
    {:ok, view, _html} = live(conn, ~p"/sandboxes/sbx_console_test/console")

    view
    |> form("form[phx-submit=run_command]", %{command: "echo hello"})
    |> render_submit()

    Process.sleep(120)
    html = render(view)

    assert html =~ "$ echo hello"
    assert html =~ "hello"
    assert html =~ "exit 0"
  end
end
