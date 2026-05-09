defmodule GardenWeb.OpenAPI do
  @moduledoc false

  def spec do
    %{
      openapi: "3.1.0",
      info: %{
        title: "Garden API",
        version: "0.1.0",
        description: "Sandbox control plane and Seed session APIs"
      },
      paths: %{
        "/api/v1/sandboxes" => %{
          get: %{summary: "List sandboxes", responses: %{"200" => json_array("Sandbox list")}},
          post: %{summary: "Acquire sandbox", requestBody: json_body(sandbox_create_schema()), responses: %{"201" => json_object("Sandbox created"), "422" => error_response()}}
        },
        "/api/v1/sandboxes/{sandbox_id}" => %{
          get: %{summary: "Get sandbox", parameters: [path_id("sandbox_id")], responses: %{"200" => json_object("Sandbox"), "404" => error_response()}}
        },
        "/api/v1/sandboxes/{sandbox_id}/lease" => %{
          post: %{summary: "Extend lease", parameters: [path_id("sandbox_id")], requestBody: json_body(lease_schema()), responses: %{"200" => json_object("Lease"), "422" => error_response()}}
        },
        "/api/v1/sandboxes/{sandbox_id}/release" => %{
          post: %{summary: "Release sandbox", parameters: [path_id("sandbox_id")], requestBody: json_body(release_schema()), responses: %{"200" => json_object("Sandbox"), "404" => error_response()}}
        },
        "/api/v1/sandboxes/{sandbox_id}/events" => %{
          get: %{summary: "Replay sandbox events", parameters: [path_id("sandbox_id"), after_param()], responses: %{"200" => json_object("Sandbox events"), "404" => error_response()}}
        },
        "/api/v1/sandboxes/{sandbox_id}/commands" => %{
          get: %{summary: "List commands", parameters: [path_id("sandbox_id")], responses: %{"200" => json_array("Commands")}},
          post: %{summary: "Start command", parameters: [path_id("sandbox_id")], requestBody: json_body(command_create_schema()), responses: %{"201" => json_object("Command"), "409" => error_response(), "422" => error_response()}}
        },
        "/api/v1/sandboxes/{sandbox_id}/commands/{command_id}" => %{
          get: %{summary: "Get command", parameters: [path_id("sandbox_id"), path_id("command_id")], responses: %{"200" => json_object("Command"), "404" => error_response()}}
        },
        "/api/v1/sandboxes/{sandbox_id}/commands/{command_id}/stdin" => %{
          post: %{summary: "Send command stdin", parameters: [path_id("sandbox_id"), path_id("command_id")], requestBody: json_body(stdin_schema()), responses: %{"200" => json_object("stdin accepted"), "422" => error_response()}}
        },
        "/api/v1/sandboxes/{sandbox_id}/commands/{command_id}/cancel" => %{
          post: %{summary: "Cancel command", parameters: [path_id("sandbox_id"), path_id("command_id")], requestBody: json_body(cancel_schema()), responses: %{"200" => json_object("Command"), "409" => error_response(), "422" => error_response()}}
        },
        "/api/v1/sandboxes/{sandbox_id}/commands/{command_id}/kill" => %{
          post: %{summary: "Kill command", parameters: [path_id("sandbox_id"), path_id("command_id")], requestBody: json_body(%{type: "object"}), responses: %{"200" => json_object("Command"), "409" => error_response()}}
        },
        "/api/v1/sandboxes/{sandbox_id}/commands/{command_id}/events" => %{
          get: %{summary: "Replay command events", parameters: [path_id("sandbox_id"), path_id("command_id"), after_param()], responses: %{"200" => json_object("Command events"), "404" => error_response()}}
        },
        "/api/openapi.json" => %{
          get: %{summary: "OpenAPI specification", responses: %{"200" => json_object("OpenAPI spec")}}
        }
      }
    }
  end

  defp json_body(schema), do: %{required: true, content: %{"application/json" => %{schema: schema}}}
  defp json_object(description), do: %{description: description, content: %{"application/json" => %{schema: %{type: "object"}}}}
  defp json_array(description), do: %{description: description, content: %{"application/json" => %{schema: %{type: "array", items: %{type: "object"}}}}}
  defp error_response, do: %{description: "Error", content: %{"application/json" => %{schema: %{type: "object", properties: %{error: %{type: "object"}}}}}}
  defp path_id(name), do: %{name: name, in: "path", required: true, schema: %{type: "string"}}
  defp after_param, do: %{name: "after", in: "query", required: false, schema: %{type: "string"}}

  defp sandbox_create_schema do
    %{type: "object", properties: %{environment: %{type: "string", enum: ["linux", "macos"]}, template: %{type: "string"}, ttl_ms: %{type: "integer"}, metadata: %{type: "object"}}}
  end

  defp lease_schema do
    %{type: "object", required: ["ttl_ms"], properties: %{ttl_ms: %{type: "integer", minimum: 1}, reason: %{type: "string"}}}
  end

  defp release_schema do
    %{type: "object", properties: %{reason: %{type: "string"}}}
  end

  defp command_create_schema do
    %{type: "object", required: ["command"], properties: %{command: %{type: "string"}, cwd: %{type: "string"}, env: %{type: "object"}, stdin: %{type: "boolean"}, timeout_ms: %{type: "integer"}, metadata: %{type: "object"}}}
  end

  defp stdin_schema do
    %{type: "object", required: ["data"], properties: %{data: %{type: "string"}, encoding: %{type: "string", enum: ["utf-8"]}}}
  end

  defp cancel_schema do
    %{type: "object", properties: %{grace_period_ms: %{type: "integer", minimum: 0}, escalation: %{type: "string", enum: ["kill"]}}}
  end
end
