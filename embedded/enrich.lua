-- enrich.lua v3.0 — Structured log enrichment for VictoriaLogs
--
-- Features:
--   JSON parse (msg/message/error/text + errmsg/error_message)
--   Numeric levels (bunyan/pino/winston: 10-70)
--   Text levels (error/warn/fatal/crit/panic/emerg)
--   Noise filter: drop_apps + drop_msgs
--   Sub-app extraction (name/component/service/module)
--   Source tag → app mapping (file inputs)

local drop_apps = {
    ["(sd-pam)"]        = true,
    ["systemd-logind"]  = true,
    ["sshd-session"]    = true,
    ["(systemd)"]       = true,
}

local drop_msgs = {
    "pam_unix%(cron:session%)",
    "Removed session",
    "New session %d+",
    "session opened for user root",
    "session closed for user root",
}

local num_to_level = {
    ["10"] = "debug", ["20"] = "debug", ["30"] = "info",
    ["40"] = "warn",  ["50"] = "error", ["60"] = "fatal", ["70"] = "fatal",
}

local text_to_level = {
    debug="debug", trace="debug",
    info="info", notice="info",
    warn="warn", warning="warn",
    error="error", err="error",
    fatal="fatal", crit="fatal", critical="fatal",
    panic="fatal", alert="fatal", emerg="fatal",
}

-- Маппинг тегов файловых input → имя приложения
local tag_to_app = {
    kerio_mail     = "kerio/mail",
    kerio_security = "kerio/security",
    kerio_error    = "kerio/error",
    kerio_warning  = "kerio/warning",
    kerio_spam     = "kerio/spam",
    nginx_access   = "nginx/access",
    nginx_error    = "nginx/error",
    apache_access  = "apache/access",
    apache_error   = "apache/error",
    postgresql     = "postgresql",
    mysql          = "mysql",
    mongodb        = "mongodb",
    redis          = "redis",
    fail2ban       = "fail2ban",
    syslog         = "syslog",
    auth           = "sshd",
}

local function json_str(s, key)
    local val = s:match('"' .. key .. '"%s*:%s*"(.-[^\\])"')
    if not val then
        if s:match('"' .. key .. '"%s*:%s*""') then return "" end
    end
    return val
end

local function json_num(s, key)
    return s:match('"' .. key .. '"%s*:%s*(%d+)')
end

local function should_drop_msg(msg)
    for _, p in ipairs(drop_msgs) do
        if msg:match(p) then return true end
    end
    return false
end

local function detect_level_text(msg, cur)
    if cur ~= "info" then return cur end
    local ml = msg:lower()
    if ml:match("%f[%a]fatal%f[%A]") or ml:match("%f[%a]panic%f[%A]")
       or ml:match("%f[%a]emerg") then
        return "fatal"
    elseif ml:match("%f[%a]error%f[%A]") or ml:match("%f[%a]failed%f[%A]")
       or ml:match("%f[%a]crit%f[%A]") then
        return "error"
    elseif ml:match("%f[%a]warn") then
        return "warn"
    end
    return cur
end

function enrich(tag, ts, r)
    -- App: из journal, из tag маппинга, или из record
    local app = r["SYSLOG_IDENTIFIER"] or r["_COMM"]
    if not app or app == "" then
        -- Файловый input: используем tag → app маппинг
        app = tag_to_app[tag] or r["app"] or tag or "unknown"
    end
    if drop_apps[app] then return -1, 0, 0 end

    -- Message
    local m = r["MESSAGE"] or r["message"] or r["log"] or r["msg"] or ""
    if m == "" then return -1, 0, 0 end
    if should_drop_msg(m) then return -1, 0, 0 end

    -- Level from syslog PRIORITY
    local p = tonumber(r["PRIORITY"] or r["pri"] or "6")
    local level = "info"
    if     p <= 2 then level = "fatal"
    elseif p == 3 then level = "error"
    elseif p == 4 then level = "warn"
    elseif p >= 7 then level = "debug" end

    -- Level from parsed field (kerio, nginx parsers)
    local parsed_level = r["level"]
    if parsed_level and parsed_level ~= "" then
        local ll = text_to_level[parsed_level:lower()]
        if ll then level = ll end
    end

    -- JSON parse (сохраняем оригинал для извлечения полей)
    if m:sub(1, 1) == "{" then
        local raw = m  -- оригинальный JSON для level/name
        local msg = json_str(raw, "msg") or json_str(raw, "message")
                 or json_str(raw, "error") or json_str(raw, "text")
        local err_msg = json_str(raw, "errmsg") or json_str(raw, "error_message")
        if msg then
            if err_msg and err_msg ~= "" and err_msg ~= msg then
                msg = msg .. " — " .. err_msg
            end
            m = msg
        end
        local lv = json_str(raw, "level") or json_num(raw, "level")
                or json_str(raw, "severity") or json_num(raw, "severity")
        if lv then
            if num_to_level[lv] then level = num_to_level[lv]
            elseif text_to_level[lv:lower()] then level = text_to_level[lv:lower()] end
        end
        local name = json_str(raw, "name") or json_str(raw, "component")
                  or json_str(raw, "service") or json_str(raw, "module")
        if name and name ~= "" then app = app .. "/" .. name end
    end

    -- Text-based level detection
    level = detect_level_text(m, level)
    if level == "fatal" then level = "error" end

    return 1, ts, {
        hostname = "FBHOST_PLACEHOLDER",
        _msg     = m,
        app      = app,
        level    = level,
        job      = "FBJOB_PLACEHOLDER",
    }
end
