package backend

import (
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"openclawdeploy/internal/shared"
	"openclawdeploy/server/swaggerui"
)

func (a *App) handleSwaggerRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/swagger/", http.StatusFound)
}

func (a *App) handleSwaggerIndex(w http.ResponseWriter, r *http.Request) {
	serveEmbeddedSwaggerFile(w, "index.html")
}

func (a *App) handleSwaggerAsset(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(strings.TrimSpace(r.PathValue("asset")))
	switch name {
	case "swagger-ui.css", "swagger-ui-bundle.js", "swagger-ui-standalone-preset.js":
		serveEmbeddedSwaggerFile(w, name)
	default:
		shared.WriteError(w, http.StatusNotFound, "swagger asset not found")
	}
}

func (a *App) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	shared.WriteJSON(w, http.StatusOK, a.openAPISpec(r))
}

func (a *App) openAPISpec(r *http.Request) map[string]any {
	return map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       "OpenClaw Deploy Server API",
			"version":     "1.0.0",
			"description": "外部程序可直接使用 serverconfig.json 中的 ai_token 调用受保护接口，无需先登录 admin/admin。",
		},
		"servers": []any{
			map[string]any{"url": a.publicBaseURL(r)},
		},
		"tags": []any{
			map[string]any{"name": "system"},
			map[string]any{"name": "auth"},
			map[string]any{"name": "devices"},
			map[string]any{"name": "admin"},
			map[string]any{"name": "client"},
		},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"ApiToken": map[string]any{
					"type":        "apiKey",
					"in":          "header",
					"name":        apiTokenHeaderName,
					"description": "使用 serverconfig.json 里的 ai_token。也支持 Authorization: Bearer <ai_token>。",
				},
				"BearerAuth": map[string]any{
					"type":         "http",
					"scheme":       "bearer",
					"bearerFormat": "Token",
					"description":  "登录后返回的会话 token；也可直接填 ai_token。",
				},
				"SessionCookie": map[string]any{
					"type": "apiKey",
					"in":   "cookie",
					"name": sessionCookieName,
				},
			},
			"schemas": map[string]any{
				"RegisterRequest":         objectSchema(fieldSchema("username", "string"), fieldSchema("email", "string"), fieldSchema("password", "string")),
				"LoginRequest":            objectSchema(fieldSchema("username", "string"), fieldSchema("password", "string")),
				"ForgotPasswordRequest":   objectSchema(fieldSchema("identifier", "string")),
				"ResetPasswordRequest":    objectSchema(fieldSchema("token", "string"), fieldSchema("new_password", "string")),
				"UpdateProfileRequest":    objectSchema(optionalFieldSchema("email", "string"), optionalFieldSchema("password", "string")),
				"BindDeviceRequest":       objectSchema(fieldSchema("device_id", "string")),
				"RemarkRequest":           objectSchema(fieldSchema("remark", "string")),
				"OpenClawConfigRequest":   objectSchema(fieldSchema("openclaw_json", "string")),
				"CreateUserRequest":       objectSchema(fieldSchema("username", "string"), fieldSchema("email", "string"), fieldSchema("password", "string"), optionalFieldSchema("is_admin", "boolean")),
				"UpdateUserRequest":       objectSchema(optionalFieldSchema("username", "string"), optionalFieldSchema("email", "string"), optionalFieldSchema("password", "string"), optionalFieldSchema("is_admin", "boolean")),
				"RegistrationSwitchInput": objectSchema(fieldSchema("enabled", "boolean")),
				"SMTPConfigInput": objectSchema(
					optionalFieldSchema("host", "string"),
					optionalFieldSchema("port", "integer"),
					optionalFieldSchema("username", "string"),
					optionalFieldSchema("password", "string"),
					optionalFieldSchema("from", "string"),
				),
				"ClientHeartbeatRequest": objectSchema(
					fieldSchema("device_id", "string"),
					fieldSchema("hostname", "string"),
					fieldSchema("system_version", "string"),
					fieldSchema("os", "string"),
					fieldSchema("arch", "string"),
					fieldSchema("local_ip", "string"),
					fieldSchema("mac", "string"),
					fieldSchema("cpu_count", "integer"),
					fieldSchema("cpu_percent", "number"),
					fieldSchema("memory_percent", "number"),
					fieldSchema("network_ok", "boolean"),
					fieldSchema("openclaw_json", "string"),
					fieldSchema("openclaw_hash", "string"),
					fieldSchema("sync_interval_seconds", "integer"),
				),
			},
		},
		"paths": map[string]any{
			"/api/v1/settings/public": map[string]any{
				"get": operation("system", "读取公开设置", nil, nil, "200"),
			},
			"/api/v1/auth/register": map[string]any{
				"post": operation("auth", "注册用户", requestRef("RegisterRequest"), nil, "201"),
			},
			"/api/v1/auth/login": map[string]any{
				"post": operation("auth", "登录并获取会话", requestRef("LoginRequest"), nil, "200"),
			},
			"/api/v1/auth/forgot-password": map[string]any{
				"post": operation("auth", "发送重置密码链接", requestRef("ForgotPasswordRequest"), nil, "200"),
			},
			"/api/v1/auth/reset-password": map[string]any{
				"post": operation("auth", "使用令牌重置密码", requestRef("ResetPasswordRequest"), nil, "200"),
			},
			"/api/v1/auth/me": map[string]any{
				"get": operation("auth", "读取当前调用身份", nil, protectedSecurity(), "200"),
			},
			"/api/v1/auth/profile": map[string]any{
				"put": operation("auth", "更新当前用户资料（邮箱/密码）", requestRef("UpdateProfileRequest"), protectedSecurity(), "200"),
			},
			"/api/v1/auth/logout": map[string]any{
				"post": operation("auth", "退出登录", nil, protectedSecurity(), "200"),
			},
			"/api/v1/devices": map[string]any{
				"get": operation("devices", "列出当前用户可见设备；管理员可按用户名筛选", nil, protectedSecurity(), "200", queryParam("owner_username", "管理员按用户名模糊筛选设备")),
			},
			"/api/v1/devices/bind": map[string]any{
				"post": operation("devices", "绑定机器识别码到当前账号", requestRef("BindDeviceRequest"), protectedSecurity(), "200"),
			},
			"/api/v1/devices/{deviceID}": map[string]any{
				"delete": operation("devices", "删除设备记录", nil, protectedSecurity(), "200", pathParam("deviceID", "机器识别码")),
			},
			"/api/v1/devices/{deviceID}/remark": map[string]any{
				"put": operation("devices", "更新设备备注", requestRef("RemarkRequest"), protectedSecurity(), "200", pathParam("deviceID", "机器识别码")),
			},
			"/api/v1/devices/{deviceID}/config": map[string]any{
				"put": operation("devices", "下发 openclaw.json", requestRef("OpenClawConfigRequest"), protectedSecurity(), "200", pathParam("deviceID", "机器识别码")),
			},
			"/api/v1/admin/summary": map[string]any{
				"get": operation("admin", "读取管理员概览", nil, protectedSecurity(), "200"),
			},
			"/api/v1/admin/settings": map[string]any{
				"get": operation("admin", "读取管理员设置", nil, protectedSecurity(), "200"),
			},
			"/api/v1/admin/users": map[string]any{
				"get":  operation("admin", "列出全部用户", nil, protectedSecurity(), "200"),
				"post": operation("admin", "创建用户", requestRef("CreateUserRequest"), protectedSecurity(), "201"),
			},
			"/api/v1/admin/users/{id}": map[string]any{
				"put":    operation("admin", "更新用户", requestRef("UpdateUserRequest"), protectedSecurity(), "200", pathParam("id", "用户 ID")),
				"delete": operation("admin", "删除用户", nil, protectedSecurity(), "200", pathParam("id", "用户 ID")),
			},
			"/api/v1/admin/settings/registration": map[string]any{
				"post": operation("admin", "开关注册功能", requestRef("RegistrationSwitchInput"), protectedSecurity(), "200"),
			},
			"/api/v1/admin/settings/smtp": map[string]any{
				"post": operation("admin", "保存 SMTP 设置", requestRef("SMTPConfigInput"), protectedSecurity(), "200"),
			},
			"/api/v1/client/heartbeat": map[string]any{
				"post": operation("client", "客户端心跳与配置拉取", requestRef("ClientHeartbeatRequest"), nil, "200"),
			},
		},
	}
}

func operation(tag, summary string, requestBody map[string]any, security []map[string][]string, successCode string, parameters ...map[string]any) map[string]any {
	responseText := "ok"
	if successCode == "201" {
		responseText = "created"
	}

	result := map[string]any{
		"tags":       []string{tag},
		"summary":    summary,
		"responses":  map[string]any{successCode: map[string]any{"description": responseText}, "400": map[string]any{"description": "bad request"}, "401": map[string]any{"description": "unauthorized"}},
		"parameters": parameters,
	}
	if requestBody != nil {
		result["requestBody"] = requestBody
	}
	if len(security) > 0 {
		result["security"] = security
	}
	return result
}

func protectedSecurity() []map[string][]string {
	return []map[string][]string{
		{"ApiToken": []string{}},
		{"BearerAuth": []string{}},
		{"SessionCookie": []string{}},
	}
}

func requestRef(schemaName string) map[string]any {
	return map[string]any{
		"required": true,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": map[string]any{
					"$ref": "#/components/schemas/" + schemaName,
				},
			},
		},
	}
}

func pathParam(name, description string) map[string]any {
	return map[string]any{
		"name":        name,
		"in":          "path",
		"required":    true,
		"description": description,
		"schema": map[string]any{
			"type": "string",
		},
	}
}

func queryParam(name, description string) map[string]any {
	return map[string]any{
		"name":        name,
		"in":          "query",
		"required":    false,
		"description": description,
		"schema": map[string]any{
			"type": "string",
		},
	}
}

func objectSchema(fields ...map[string]any) map[string]any {
	properties := map[string]any{}
	required := make([]string, 0, len(fields))
	for _, field := range fields {
		name := strings.TrimSpace(field["_name"].(string))
		requiredFlag, _ := field["_required"].(bool)
		delete(field, "_name")
		delete(field, "_required")
		properties[name] = field
		if requiredFlag {
			required = append(required, name)
		}
	}

	result := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		result["required"] = required
	}
	return result
}

func fieldSchema(name, fieldType string) map[string]any {
	return map[string]any{
		"_name":     name,
		"_required": true,
		"type":      fieldType,
	}
}

func optionalFieldSchema(name, fieldType string) map[string]any {
	return map[string]any{
		"_name":     name,
		"_required": false,
		"type":      fieldType,
	}
}

func serveEmbeddedSwaggerFile(w http.ResponseWriter, name string) {
	data, err := fs.ReadFile(swaggerui.StaticFS(), name)
	if err != nil {
		shared.WriteError(w, http.StatusNotFound, "swagger asset not found")
		return
	}

	if contentType := mime.TypeByExtension(filepath.Ext(name)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
