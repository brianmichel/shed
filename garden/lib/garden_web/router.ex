defmodule GardenWeb.Router do
  use GardenWeb, :router

  pipeline :browser do
    plug :accepts, ["html"]
    plug :fetch_session
    plug :fetch_live_flash
    plug :put_root_layout, html: {GardenWeb.Layouts, :root}
    plug :protect_from_forgery
    plug :put_secure_browser_headers
  end

  pipeline :api do
    plug :accepts, ["json"]
  end

  scope "/", GardenWeb do
    pipe_through :browser

    get "/", PageController, :home
    get "/api/openapi.json", OpenAPIController, :show
    live "/seed/sessions", SeedSessionLive.Index, :index
    live "/seed/sessions/:session_id", SeedSessionLive.Show, :show
    live "/sandboxes/:sandbox_id/console", SandboxConsoleLive.Show, :show
  end

  scope "/api/v1", GardenWeb.Api.V1 do
    pipe_through :api

    resources "/sandboxes", SandboxController, only: [:index, :create, :show], param: "sandbox_id" do
      post "/release", SandboxController, :release
      post "/lease", SandboxController, :lease
      get "/events", SandboxController, :events

      get "/files", FileController, :index
      get "/files/content", FileController, :show
      put "/files/content", FileController, :update

      resources "/commands", CommandController, only: [:index, :create, :show], param: "command_id" do
        post "/stdin", CommandController, :stdin
        post "/cancel", CommandController, :cancel
        post "/kill", CommandController, :kill
        get "/events", CommandController, :events
      end
    end
  end

  # Enable LiveDashboard and Swoosh mailbox preview in development
  if Application.compile_env(:garden, :dev_routes) do
    # If you want to use the LiveDashboard in production, you should put
    # it behind authentication and allow only admins to access it.
    # If your application does not have an admins-only section yet,
    # you can use Plug.BasicAuth to set up some basic authentication
    # as long as you are also using SSL (which you should anyway).
    import Phoenix.LiveDashboard.Router

    scope "/dev" do
      pipe_through :browser

      live_dashboard "/dashboard", metrics: GardenWeb.Telemetry
      forward "/mailbox", Plug.Swoosh.MailboxPreview
    end
  end
end
