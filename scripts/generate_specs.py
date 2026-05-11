#!/usr/bin/env python3
from __future__ import annotations

import json
import re
import sys
import textwrap
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parent.parent
PROTOCOL_SPEC = ROOT / "spec/protocol/messages.json"
API_SPEC = ROOT / "spec/api/openapi.json"
GARDEN_PROTOCOL_DIR = ROOT / "garden/lib/garden/seed_protocol"
GARDEN_CAPS_DIR = GARDEN_PROTOCOL_DIR / "capabilities"
GARDEN_OPENAPI_GEN = ROOT / "garden/lib/garden_web/generated_open_api.ex"
SEED_PROTOCOL_GEN = ROOT / "seed/internal/protocol/generated.go"
GARDEN_REGISTRY_GEN = GARDEN_PROTOCOL_DIR / "registry.ex"

TYPE_MAP = {
    "string": ":string",
    "integer": ":integer",
    "float": ":float",
    "boolean": ":boolean",
    "map": ":map",
}


def load_json(path: Path) -> Any:
    return json.loads(path.read_text())


def camelize(value: str) -> str:
    return "".join(part.capitalize() for part in re.split(r"[^a-zA-Z0-9]+", value) if part)


def snake_constant(value: str) -> str:
    out = re.sub(r"[^a-zA-Z0-9]+", "_", value).strip("_")
    return out.upper()


def elixir_key(key: str) -> str:
    if re.match(r"^[a-z_][a-z0-9_]*$", key):
        return key
    return f'"{key}"'


def elixir_literal(value: Any, indent: int = 0) -> str:
    space = " " * indent
    next_space = " " * (indent + 2)
    if isinstance(value, dict):
        if not value:
            return "%{}"
        items = []
        for k, v in value.items():
            items.append(f"{next_space}{elixir_key(str(k))}: {elixir_literal(v, indent + 2)}")
        return "%{\n" + ",\n".join(items) + f"\n{space}}}"
    if isinstance(value, list):
        if not value:
            return "[]"
        items = [f"{next_space}{elixir_literal(v, indent + 2)}" for v in value]
        return "[\n" + ",\n".join(items) + f"\n{space}]"
    if isinstance(value, str):
        return json.dumps(value)
    if value is True:
        return "true"
    if value is False:
        return "false"
    if value is None:
        return "nil"
    return str(value)


def field_schema(message: dict[str, Any]) -> dict[str, Any] | None:
    payload = message.get("payload") or {}
    fields = payload.get("fields") or []
    if not fields:
        return None

    schema: dict[str, Any] = {"fields": []}
    required = []
    inclusion = {}

    for field in fields:
        schema["fields"].append((field["name"], TYPE_MAP[field["type"]]))
        if field.get("required"):
            required.append(field["name"])
        if field.get("enum"):
            inclusion[field["name"]] = field["enum"]

    if required:
        schema["required"] = required
    if inclusion:
        schema["inclusion"] = inclusion
    return schema


def render_field_keyword_list(fields: list[tuple[str, str]]) -> str:
    return "[" + ", ".join(f"{name}: {type_name}" for name, type_name in fields) + "]"


def render_schema_map(messages: list[dict[str, Any]]) -> str:
    entries = []
    for message in messages:
        schema = field_schema(message)
        if not schema:
            continue
        parts = [f"fields: {render_field_keyword_list(schema['fields'])}"]
        if schema.get("required"):
            req = ", ".join(f":{name}" for name in schema["required"])
            parts.append(f"required: [{req}]")
        if schema.get("inclusion"):
            inclusion_items = []
            for key, values in schema["inclusion"].items():
                quoted = ", ".join(json.dumps(v) for v in values)
                inclusion_items.append(f"{key}: [{quoted}]")
            parts.append("inclusion: %{" + ", ".join(inclusion_items) + "}")
        entries.append(f'      {json.dumps(message["type"])} => %{{' + ", ".join(parts) + "}")
    if not entries:
        return "%{}"
    return "%{\n" + ",\n".join(entries) + "\n    }"


def render_messages(messages: list[dict[str, Any]]) -> str:
    if not messages:
        return "[]"
    lines = ["["]
    for idx, message in enumerate(messages):
        lines.append(f"      # {message['doc']}")
        suffix = "," if idx < len(messages) - 1 else ""
        lines.append(f'      {json.dumps(message["type"])}{suffix}')
    lines.append("    ]")
    return "\n".join(lines)


def capability_module_name(name: str) -> str:
    return camelize(name)


def generate_capability_module(capability: dict[str, Any]) -> str:
    mod = capability_module_name(capability["name"])
    inbound = [m for m in capability["messages"] if m["direction"] == "inbound"]
    outbound = [m for m in capability["messages"] if m["direction"] == "outbound"]
    bidi = [m for m in capability["messages"] if m["direction"] == "bidirectional"]
    payload_map = render_schema_map(capability["messages"])

    sections = []
    if inbound:
        sections.append(
            "  @impl true\n"
            "  def inbound_messages do\n"
            f"{textwrap.indent(render_messages(inbound), '    ')}\n"
            "  end\n"
        )
    if outbound:
        sections.append(
            "  @impl true\n"
            "  def outbound_messages do\n"
            f"{textwrap.indent(render_messages(outbound), '    ')}\n"
            "  end\n"
        )
    if bidi:
        sections.append(
            "  @impl true\n"
            "  def bidirectional_messages do\n"
            f"{textwrap.indent(render_messages(bidi), '    ')}\n"
            "  end\n"
        )

    return f'''defmodule Garden.SeedProtocol.Capabilities.{mod} do
  @moduledoc """
  {capability["description"]}
  """

  use Garden.SeedProtocol.Capability

  @impl true
  def name, do: :{capability["name"]}

{chr(10).join(sections)}  @impl true
  def payload_schemas do
    {payload_map}
  end
end
'''


def generate_registry(protocol: dict[str, Any]) -> str:
    modules = ",\n    ".join(
        f"Capabilities.{capability_module_name(cap['name'])}" for cap in protocol["capabilities"]
    )
    return f'''defmodule Garden.SeedProtocol.Registry do
  @moduledoc """
  Central registry for protocol capability modules.

  This file is generated from `spec/protocol/messages.json`.
  """

  alias Garden.SeedProtocol.Capabilities

  @capabilities [
    {modules}
  ]

  def capabilities, do: @capabilities

  def all_types do
    @capabilities
    |> Enum.flat_map(& &1.message_types())
    |> Enum.uniq()
  end

  def schema_for(type) do
    Enum.find_value(@capabilities, fn mod ->
      Map.get(mod.payload_schemas(), type)
    end)
  end

  def capability_for(type) do
    Enum.find(@capabilities, fn mod -> type in mod.message_types() end)
  end
end
'''


def generate_openapi_module(spec: dict[str, Any]) -> str:
    json_blob = json.dumps(spec, indent=2)
    return f'''defmodule GardenWeb.GeneratedOpenAPI do
  @moduledoc false

  def spec do
    Jason.decode!(~S"""
{json_blob}
""")
  end
end
'''


def go_field_type(kind: str) -> str:
    return {
        "string": "string",
        "integer": "integer",
        "float": "float",
        "boolean": "boolean",
        "map": "map",
    }[kind]


def generate_go_protocol(protocol: dict[str, Any]) -> str:
    constants = [f'\tVersion = {json.dumps(protocol["protocol_version"])}']
    specs = []

    for capability in protocol["capabilities"]:
        for message in capability["messages"]:
            const_name = camelize(message["type"])
            constants.append(f'\t{const_name} = {json.dumps(message["type"])}')

            payload = message.get("payload") or {}
            fields = payload.get("fields") or []
            field_entries = []
            for field in fields:
                enum_values = ", ".join(json.dumps(v) for v in field.get("enum", []))
                enum_part = f"[]string{{{enum_values}}}" if enum_values else "nil"
                field_entries.append(
                    f'\t\t\t{json.dumps(field["name"])}: {{Type: {json.dumps(go_field_type(field["type"]))}, Required: {str(bool(field.get("required"))).lower()}, Enum: {enum_part}}},'
                )
            field_block = "\n".join(field_entries)
            specs.append(
                f'\t{json.dumps(message["type"])}: {{Capability: {json.dumps(capability["name"])}, Direction: {json.dumps(message["direction"])}, Fields: map[string]FieldSpec{{\n{field_block}\n\t\t}}}},'
            )

    return f'''// Code generated by scripts/generate_specs.py. DO NOT EDIT.
package protocol

import (
\t"fmt"
\t"math"
)

const (
{chr(10).join(constants)}
)

type FieldSpec struct {{
\tType     string
\tRequired bool
\tEnum     []string
}}

type MessageSpec struct {{
\tCapability string
\tDirection  string
\tFields     map[string]FieldSpec
}}

var MessageSpecs = map[string]MessageSpec{{
{chr(10).join(specs)}
}}

func ValidatePayload(msgType string, payload map[string]any) error {{
\tspec, ok := MessageSpecs[msgType]
\tif !ok {{
\t\treturn fmt.Errorf("unknown message type: %s", msgType)
\t}}
\tfor name, field := range spec.Fields {{
\t\tvalue, present := payload[name]
\t\tif field.Required && !present {{
\t\t\treturn fmt.Errorf("%s missing required field %s", msgType, name)
\t\t}}
\t\tif !present {{
\t\t\tcontinue
\t\t}}
\t\tif !matchesType(field.Type, value) {{
\t\t\treturn fmt.Errorf("%s field %s must be %s", msgType, name, field.Type)
\t\t}}
\t\tif len(field.Enum) > 0 {{
\t\t\ts, ok := value.(string)
\t\t\tif !ok {{
\t\t\t\treturn fmt.Errorf("%s field %s must be string enum", msgType, name)
\t\t\t}}
\t\t\tmatched := false
\t\t\tfor _, allowed := range field.Enum {{
\t\t\t\tif s == allowed {{
\t\t\t\t\tmatched = true
\t\t\t\t\tbreak
\t\t\t\t}}
\t\t\t}}
\t\t\tif !matched {{
\t\t\t\treturn fmt.Errorf("%s field %s has invalid value %q", msgType, name, s)
\t\t\t}}
\t\t}}
\t}}
\treturn nil
}}

func matchesType(kind string, value any) bool {{
\tswitch kind {{
\tcase "string":
\t\t_, ok := value.(string)
\t\treturn ok
\tcase "boolean":
\t\t_, ok := value.(bool)
\t\treturn ok
\tcase "map":
\t\tswitch value.(type) {{
\t\tcase map[string]any, map[string]string:
\t\t\treturn true
\t\tdefault:
\t\t\treturn false
\t\t}}
\tcase "float":
\t\tswitch value.(type) {{
\t\tcase float64, float32:
\t\t\treturn true
\t\tdefault:
\t\t\treturn false
\t\t}}
\tcase "integer":
\t\tswitch v := value.(type) {{
\t\tcase int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
\t\t\treturn true
\t\tcase float64:
\t\t\treturn math.Trunc(v) == v
\t\tcase float32:
\t\t\treturn float32(math.Trunc(float64(v))) == v
\t\tdefault:
\t\t\treturn false
\t\t}}
\tdefault:
\t\treturn true
\t}}
}}
'''


def write_if_changed(path: Path, content: str, check: bool) -> bool:
    existing = path.read_text() if path.exists() else None
    if existing == content:
        return False
    if check:
        print(f"stale: {path.relative_to(ROOT)}")
        return True
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content)
    print(f"wrote {path.relative_to(ROOT)}")
    return True


def generate_protocol(check: bool = False) -> bool:
    protocol = load_json(PROTOCOL_SPEC)
    changed = False
    for capability in protocol["capabilities"]:
        mod_name = capability["name"]
        path = GARDEN_CAPS_DIR / f"{mod_name}.ex"
        changed |= write_if_changed(path, generate_capability_module(capability), check)
    changed |= write_if_changed(GARDEN_REGISTRY_GEN, generate_registry(protocol), check)
    changed |= write_if_changed(SEED_PROTOCOL_GEN, generate_go_protocol(protocol), check)
    return changed


def generate_api(check: bool = False) -> bool:
    spec = load_json(API_SPEC)
    return write_if_changed(GARDEN_OPENAPI_GEN, generate_openapi_module(spec), check)


def main() -> int:
    cmd = sys.argv[1] if len(sys.argv) > 1 else "all"
    check = cmd == "check"
    changed = False

    if cmd in {"all", "protocol", "check"}:
        changed |= generate_protocol(check)
    if cmd in {"all", "api", "check"}:
        changed |= generate_api(check)

    if check and changed:
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
