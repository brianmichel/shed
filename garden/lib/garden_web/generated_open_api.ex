defmodule GardenWeb.GeneratedOpenAPI do
  @moduledoc false

  def spec do
    Jason.decode!(~S"""
{
  "openapi": "3.1.0",
  "info": {
    "title": "Garden API",
    "version": "0.1.0",
    "description": "Sandbox control plane and Seed session APIs"
  },
  "paths": {
    "/api/v1/sandboxes": {
      "get": {
        "summary": "List sandboxes",
        "responses": {
          "200": {
            "description": "Sandbox list",
            "content": {
              "application/json": {
                "schema": {
                  "type": "array",
                  "items": {
                    "type": "object"
                  }
                }
              }
            }
          }
        }
      },
      "post": {
        "summary": "Acquire sandbox",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "environment": {
                    "type": "string",
                    "enum": [
                      "linux",
                      "macos"
                    ]
                  },
                  "template": {
                    "type": "string"
                  },
                  "ttl_ms": {
                    "type": "integer"
                  },
                  "metadata": {
                    "type": "object"
                  }
                }
              }
            }
          }
        },
        "responses": {
          "201": {
            "description": "Sandbox created",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "422": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "error": {
                      "type": "object"
                    }
                  }
                }
              }
            }
          }
        }
      }
    },
    "/api/v1/sandboxes/{sandbox_id}": {
      "get": {
        "summary": "Get sandbox",
        "parameters": [
          {
            "name": "sandbox_id",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Sandbox",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "404": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "error": {
                      "type": "object"
                    }
                  }
                }
              }
            }
          }
        }
      }
    },
    "/api/v1/sandboxes/{sandbox_id}/lease": {
      "post": {
        "summary": "Extend lease",
        "parameters": [
          {
            "name": "sandbox_id",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string"
            }
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": [
                  "ttl_ms"
                ],
                "properties": {
                  "ttl_ms": {
                    "type": "integer",
                    "minimum": 1
                  },
                  "reason": {
                    "type": "string"
                  }
                }
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Lease",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "422": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "error": {
                      "type": "object"
                    }
                  }
                }
              }
            }
          }
        }
      }
    },
    "/api/v1/sandboxes/{sandbox_id}/release": {
      "post": {
        "summary": "Release sandbox",
        "parameters": [
          {
            "name": "sandbox_id",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string"
            }
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "reason": {
                    "type": "string"
                  }
                }
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Sandbox",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "404": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "error": {
                      "type": "object"
                    }
                  }
                }
              }
            }
          }
        }
      }
    },
    "/api/v1/sandboxes/{sandbox_id}/events": {
      "get": {
        "summary": "Replay sandbox events",
        "parameters": [
          {
            "name": "sandbox_id",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string"
            }
          },
          {
            "name": "after",
            "in": "query",
            "required": false,
            "schema": {
              "type": "string"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Sandbox events",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "404": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "error": {
                      "type": "object"
                    }
                  }
                }
              }
            }
          }
        }
      }
    },
    "/api/v1/sandboxes/{sandbox_id}/commands": {
      "get": {
        "summary": "List commands",
        "parameters": [
          {
            "name": "sandbox_id",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Commands",
            "content": {
              "application/json": {
                "schema": {
                  "type": "array",
                  "items": {
                    "type": "object"
                  }
                }
              }
            }
          }
        }
      },
      "post": {
        "summary": "Start command",
        "parameters": [
          {
            "name": "sandbox_id",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string"
            }
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": [
                  "command"
                ],
                "properties": {
                  "command": {
                    "type": "string"
                  },
                  "cwd": {
                    "type": "string"
                  },
                  "env": {
                    "type": "object"
                  },
                  "stdin": {
                    "type": "boolean"
                  },
                  "timeout_ms": {
                    "type": "integer"
                  },
                  "metadata": {
                    "type": "object"
                  }
                }
              }
            }
          }
        },
        "responses": {
          "201": {
            "description": "Command",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "409": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "error": {
                      "type": "object"
                    }
                  }
                }
              }
            }
          },
          "422": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "error": {
                      "type": "object"
                    }
                  }
                }
              }
            }
          }
        }
      }
    },
    "/api/v1/sandboxes/{sandbox_id}/commands/{command_id}": {
      "get": {
        "summary": "Get command",
        "parameters": [
          {
            "name": "sandbox_id",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string"
            }
          },
          {
            "name": "command_id",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Command",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "404": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "error": {
                      "type": "object"
                    }
                  }
                }
              }
            }
          }
        }
      }
    },
    "/api/v1/sandboxes/{sandbox_id}/commands/{command_id}/stdin": {
      "post": {
        "summary": "Send command stdin",
        "parameters": [
          {
            "name": "sandbox_id",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string"
            }
          },
          {
            "name": "command_id",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string"
            }
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": [
                  "data"
                ],
                "properties": {
                  "data": {
                    "type": "string"
                  },
                  "encoding": {
                    "type": "string",
                    "enum": [
                      "utf-8"
                    ]
                  }
                }
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "stdin accepted",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "422": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "error": {
                      "type": "object"
                    }
                  }
                }
              }
            }
          }
        }
      }
    },
    "/api/v1/sandboxes/{sandbox_id}/commands/{command_id}/cancel": {
      "post": {
        "summary": "Cancel command",
        "parameters": [
          {
            "name": "sandbox_id",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string"
            }
          },
          {
            "name": "command_id",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string"
            }
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "grace_period_ms": {
                    "type": "integer",
                    "minimum": 0
                  },
                  "escalation": {
                    "type": "string",
                    "enum": [
                      "kill"
                    ]
                  }
                }
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Command",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "409": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "error": {
                      "type": "object"
                    }
                  }
                }
              }
            }
          },
          "422": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "error": {
                      "type": "object"
                    }
                  }
                }
              }
            }
          }
        }
      }
    },
    "/api/v1/sandboxes/{sandbox_id}/commands/{command_id}/kill": {
      "post": {
        "summary": "Kill command",
        "parameters": [
          {
            "name": "sandbox_id",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string"
            }
          },
          {
            "name": "command_id",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string"
            }
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object"
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Command",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "409": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "error": {
                      "type": "object"
                    }
                  }
                }
              }
            }
          }
        }
      }
    },
    "/api/v1/sandboxes/{sandbox_id}/commands/{command_id}/events": {
      "get": {
        "summary": "Replay command events",
        "parameters": [
          {
            "name": "sandbox_id",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string"
            }
          },
          {
            "name": "command_id",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string"
            }
          },
          {
            "name": "after",
            "in": "query",
            "required": false,
            "schema": {
              "type": "string"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Command events",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "404": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "error": {
                      "type": "object"
                    }
                  }
                }
              }
            }
          }
        }
      }
    },
    "/api/openapi.json": {
      "get": {
        "summary": "OpenAPI specification",
        "responses": {
          "200": {
            "description": "OpenAPI spec",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          }
        }
      }
    }
  }
}
""")
  end
end
