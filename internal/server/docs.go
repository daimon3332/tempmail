package server

import "net/http"

// apiDocsHTML is a self-contained API documentation page served at /docs/api.
// It follows the CLIProxyAPI docs style: base path, authentication, request /
// response conventions (with curl + JSON examples), and per-endpoint notes.
const apiDocsHTML = `<!doctype html>
<html lang="zh"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Tempmail API 文档</title>
<style>
:root{--bg:#0b0f1a;--card:#121826;--line:#1f2a3d;--tx:#e6edf3;--mut:#8b98ab;--acc:#38bdf8;--code:#0a0e16;--ok:#34d399}
*{box-sizing:border-box}body{margin:0;font:16px/1.6 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"PingFang SC","Microsoft YaHei",sans-serif;background:var(--bg);color:var(--tx)}
.wrap{max-width:1000px;margin:auto;padding:32px 20px}a{color:var(--acc);text-decoration:none}
h1{font-size:28px;border-bottom:1px solid var(--line);padding-bottom:16px}h2{font-size:22px;margin-top:48px;border-bottom:1px solid var(--line);padding-bottom:8px}
h3{font-size:17px;margin-top:28px;color:var(--acc)}
code,.meth{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace}
.method{display:inline-block;font-size:12px;font-weight:700;padding:2px 8px;border-radius:5px;margin-right:8px}
.get{background:#1f6feb33;color:#79c0ff}.post{background:#23863633;color:#3fb950}.del{background:#f8514926;color:#f85149}
table{border-collapse:collapse;width:100%;margin:12px 0}th,td{border:1px solid var(--line);padding:8px 12px;text-align:left;font-size:14px}
th{background:var(--card)}pre{background:var(--code);border:1px solid var(--line);border-radius:8px;padding:14px;overflow-x:auto;font-size:13px}
pre b{color:var(--ok)}
.note{background:var(--card);border-left:3px solid var(--acc);padding:10px 14px;border-radius:0 8px 8px 0;margin:12px 0;font-size:14px;color:var(--mut)}
.note b{color:var(--tx)}
.toc{background:var(--card);border:1px solid var(--line);border-radius:8px;padding:16px 20px;columns:2;font-size:14px}
.toc a{display:block;padding:3px 0}
code.inline{background:var(--card);padding:2px 6px;border-radius:4px;font-size:13px}
@media(max-width:640px){.toc{columns:1}}
</style></head><body><div class="wrap">
<h1>Tempmail API 文档</h1>
<p>自建临时邮箱服务的 HTTP API 说明。所有接口均为 REST，JSON 请求/响应（另有说明除外）。</p>
<p>基础路径：<code class="inline">https://tempmail.333186.xyz</code></p>

<div class="toc">
<a href="#auth">认证</a><a href="#conv">请求/响应约定</a><a href="#health">健康检查</a>
<a href="#addr">地址</a><a href="#mail">邮件</a><a href="#send">发信</a><a href="#user">用户</a>
<a href="#admin">管理员</a><a href="#role">角色与配额</a><a href="#telegram">Telegram</a>
<a href="#agent">Agent 接入</a><a href="#err">错误与注意事项</a>
</div>

<h2 id="auth">认证</h2>
<p>大多数接口需要鉴权。四类凭证：</p>
<table><tr><th>类型</th><th>Header</th><th>说明</th></tr>
<tr><td>地址 JWT</td><td><code class="inline">Authorization: Bearer &lt;JWT&gt;</code></td><td><code class="inline">/api/*</code> 使用，标示"以该邮箱身份操作"</td></tr>
<tr><td>用户 JWT</td><td><code class="inline">x-user-token</code></td><td><code class="inline">/user_api/*</code> 使用</td></tr>
<tr><td>管理员密码</td><td><code class="inline">x-admin-auth</code></td><td><code class="inline">/admin/*</code> 使用</td></tr>
<tr><td>站点密码（可选）</td><td><code class="inline">x-custom-auth</code></td><td>仅当站点启用了自定义访问密码时要求</td></tr>
</table>
<div class="note"><b>注意：</b>Address JWT 与 User JWT 不可混用，否则返回 <b>401 Invalid address credential</b>。<br>
若在「配置页」设置了 <code class="inline">API Key</code>，第三方调用 <code class="inline">/api/*</code> 也可用 <code class="inline">x-api-key</code> 或 <code class="inline">Authorization: Bearer</code> 提供。</div>

<h2 id="conv">请求/响应约定</h2>
<ul>
<li>Content-Type：<code class="inline">application/json</code>（另有说明除外）。</li>
<li>通用分页参数：<code class="inline">limit</code>（1..100，默认 20）、<code class="inline">offset</code>（>=0，默认 0）。</li>
<li>标准响应：<code class="inline">{"success": true}</code>；错误为 <code class="inline">text/plain</code> 状态码文案。</li>
<li>可选的 <code class="inline">x-lang: zh|en</code> 控制错误提示语言。</li>
</ul>

<h2 id="health">健康检查</h2>
<p><span class="method get">GET</span><code class="inline">/health</code>、<code class="inline">/health_check</code></p>
<pre><b>curl</b> -s https://tempmail.333186.xyz/health
# → {"database":"ok","mail_receiver":"ok","status":"ok","version":"v1.12.0"}</pre>
<p>无需鉴权。返回 <code class="inline">200</code> 表示服务正常。</p>

<h2 id="addr">地址</h2>
<h3>创建地址</h3>
<p><span class="method post">POST</span><code class="inline">/api/new_address</code>（需站点允许；匿名或已登录用户均可，取决于配置）</p>
<pre><b>curl</b> -X POST https://tempmail.333186.xyz/api/new_address \
  -H "Content-Type: application/json" -d '{"name":"mybox","domain":"333186.xyz"}'
# → {"jwt":"eyJ...","address":"orxmybox@333186.xyz","password":null,"address_id":12345}</pre>
<div class="note"><b>说明：</b>name 留空则随机生成；<code class="inline">enableRandomSubdomain</code> 可开启随机子域。返回的 <code class="inline">jwt</code> 用于后续 <code class="inline">/api/*</code> 请求。</div>
<h3>地址信息</h3>
<p><span class="method get">GET</span><code class="inline">/api/settings</code></p>
<pre><b>curl</b> -s https://tempmail.333186.xyz/api/settings -H "Authorization: Bearer $JWT"
# → {"address":"orxmybox@333186.xyz","send_balance":0}</pre>
<h3>管理员创建 / 复用（幂等）</h3>
<p><span class="method post">POST</span><code class="inline">/admin/ensure_address</code>（需管理员）</p>
<pre><b>curl</b> -X POST https://tempmail.333186.xyz/admin/ensure_address \
  -H "x-admin-auth: $ADMIN" -H "Content-Type: application/json" -d '{"name":"helper","domain":"333186.xyz"}'
# → {"address":"helper@333186.xyz","address_id":9,"jwt":"eyJ...","reused":false}</pre>
<p><span class="method get">GET</span><code class="inline">/admin/address/lookup?address=helper@333186.xyz</code> — 精确查找。<br>
<span class="method post">POST</span><code class="inline">/admin/address/access</code> — 用 <code class="inline">{"address":"..."}</code> 或 <code class="inline">{"address_id":9}</code> 拿到该地址 JWT。</p>

<h2 id="mail">邮件</h2>
<p>浏览器前端使用服务端解析接口，返回 subject/text/html/attachments（无需自己解析 MIME）。</p>
<table><tr><th>任务</th><th>方法</th><th>路径</th></tr>
<tr><td>列出已解析邮件</td><td>GET</td><td><code class="inline">/api/parsed_mails?limit=&amp;offset=</code></td></tr>
<tr><td>取单封已解析</td><td>GET</td><td><code class="inline">/api/parsed_mail/:id</code></td></tr>
<tr><td>列原始邮件</td><td>GET</td><td><code class="inline">/api/mails?limit=&amp;offset=&amp;after_id=</code></td></tr>
<tr><td>取原始邮件（含 raw）</td><td>GET</td><td><code class="inline">/api/mail/:id</code></td></tr>
<tr><td>标记已读</td><td>POST</td><td><code class="inline">/api/mails/:id/read</code> <code class="inline">{"isUnread":false}</code></td></tr>
<tr><td>删除邮件</td><td>DELETE</td><td><code class="inline">/api/mails/:id</code></td></tr>
</table>
<pre><b>curl</b> -s "https://tempmail.333186.xyz/api/parsed_mails?limit=20&offset=0" \
  -H "Authorization: Bearer $JWT"
# → {"results":[{"id":42,"sender":"Foo <a@b.com>","subject":"Your code is 123456","text":"...","html":"<p>...</p>","attachments":[{"filename":"a.pdf","size":12345}]}],"count":1}</pre>
<div class="note"><b>说明：</b>增量拉取用 <code class="inline">after_id</code>（只返回该 id 之后的新邮件）或 <code class="inline">after=&lt;RFC3339&gt;</code>。附件在 parsed 响应里仅元数据；需要字节用 <code class="inline">/api/mail/:id</code> 取 <code class="inline">raw</code> 自行解析。</div>

<h2 id="send">发信</h2>
<table><tr><th>任务</th><th>方法</th><th>路径</th></tr>
<tr><td>申请发信权限</td><td>POST</td><td><code class="inline">/api/request_send_mail_access</code></td></tr>
<tr><td>发送邮件</td><td>POST</td><td><code class="inline">/api/send_mail</code></td></tr>
<tr><td>列出已发送</td><td>GET</td><td><code class="inline">/api/sendbox?limit=&amp;offset=</code></td></tr>
<tr><td>删除已发送</td><td>DELETE</td><td><code class="inline">/api/sendbox/:id</code></td></tr>
</table>
<pre><b>curl</b> -X POST https://tempmail.333186.xyz/api/send_mail \
  -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" \
  -d '{"from_name":"","to_mail":"someone@example.com","to_name":"","subject":"Test","content":"Hello","is_html":false}'
# → {"status":"ok"}</pre>
<div class="note"><b>说明：</b>发送需要 <code class="inline">send_balance &gt; 0</code>（见 <code class="inline">/api/settings</code>）。若部署了发件限流，超额返回 400。</div>

<h2 id="user">用户</h2>
<table><tr><th>任务</th><th>方法</th><th>路径</th><th>说明</th></tr>
<tr><td>注册</td><td>POST</td><td><code class="inline">/user_api/register</code></td><td><code class="inline">{"email","password","code","cf_token"}</code></td></tr>
<tr><td>登录</td><td>POST</td><td><code class="inline">/user_api/login</code></td><td><code class="inline">{"email","password"}</code> → <code class="inline">{"jwt"}</code></td></tr>
<tr><td>个人信息</td><td>GET</td><td><code class="inline">/user_api/settings</code></td><td>含角色、配额、access_token</td></tr>
<tr><td>绑定地址</td><td>POST</td><td><code class="inline">/user_api/bind_address</code></td><td>需 Address JWT（Bearer）</td></tr>
<tr><td>已绑地址</td><td>GET</td><td><code class="inline">/user_api/bind_address</code></td><td></td></tr>
<tr><td>Passkey 登录</td><td>POST</td><td><code class="inline">/user_api/passkey/authenticate_request|response</code></td><td>WebAuthn</td></tr>
</table>
<div class="note"><b>说明：</b>密码按 SHA-256 十六进制存储与比较，客户端需先哈希。</div>

<h2 id="admin">管理员</h2>
<p>以下均需 <code class="inline">x-admin-auth: &lt;ADMIN&gt;</code>。</p>
<table><tr><th>功能</th><th>方法</th><th>路径</th></tr>
<tr><td>创建地址</td><td>POST</td><td><code class="inline">/admin/new_address</code></td></tr>
<tr><td>地址列表</td><td>GET</td><td><code class="inline">/admin/address?query=&amp;limit=&amp;offset=&amp;sort_by=&amp;sort_order=</code></td></tr>
<tr><td>删除 / 清空</td><td>DELETE</td><td><code class="inline">/admin/delete_address/:id</code>、<code class="inline">/admin/clear_inbox/:id</code>、<code class="inline">/admin/clear_sent_items/:id</code></td></tr>
<tr><td>邮件列表</td><td>GET</td><td><code class="inline">/admin/mails?address=&amp;limit=&amp;offset=</code></td></tr>
<tr><td>未知邮件</td><td>GET</td><td><code class="inline">/admin/mails_unknow</code></td></tr>
<tr><td>用户列表</td><td>GET</td><td><code class="inline">/admin/users</code></td></tr>
<tr><td>创建 / 删除 / 重置密码</td><td>POST/DELETE</td><td><code class="inline">/admin/users</code>、<code class="inline">/admin/users/:id</code>、<code class="inline">/admin/users/:id/reset_password</code></td></tr>
<tr><td>统计</td><td>GET</td><td><code class="inline">/admin/stats</code></td></tr>
<tr><td>数据库备份</td><td>GET</td><td><code class="inline">/admin/db_backup</code>（下载 .db）</td></tr>
<tr><td>导入 .sql</td><td>POST</td><td><code class="inline">/admin/db_import?merge=1</code></td></tr>
<tr><td>地址 CSV 导出/导入</td><td>GET/POST</td><td><code class="inline">/admin/address/export</code>、<code class="inline">/admin/address/import</code></td></tr>
<tr><td>操作日志</td><td>GET/DELETE</td><td><code class="inline">/admin/operation_log</code></td></tr>
<tr><td>配置页（热更新）</td><td>GET/POST</td><td><code class="inline">/admin/runtime_config</code></td></tr>
<tr><td>系统状态</td><td>GET</td><td><code class="inline">/admin/system_status</code></td></tr>
</table>

<h2 id="role">角色与配额</h2>
<p><span class="method get">GET</span><code class="inline">/admin/roles</code>、<span class="method post">POST</span><code class="inline">/admin/roles</code>、<span class="method del">DELETE</span><code class="inline">/admin/roles/:role</code></p>
<pre><b>curl</b> -X POST https://tempmail.333186.xyz/admin/roles \
  -H "x-admin-auth: $ADMIN" -H "Content-Type: application/json" \
  -d '{"role":"premium","name":"高级","domains":["333186.xyz"],"max_address_count":10,"monthly_address_quota":100,"can_custom_name":true,"can_send_mail":true}'
# → {"success":true}
# GET /admin/roles → {"results":[{"role":"premium","max_address_count":10,"monthly_address_quota":100,"source":"db"}]}</pre>
<p><span class="method post">POST</span><code class="inline">/admin/user_roles</code> — 给用户分配角色：<code class="inline">{"user_id":1,"role_text":"premium"}</code>。</p>

<h2 id="telegram">Telegram</h2>
<table><tr><th>功能</th><th>方法</th><th>路径</th></tr>
<tr><td>Webhook 入口</td><td>POST</td><td><code class="inline">/telegram/webhook</code>（Telegram 服务器调用）</td></tr>
<tr><td>初始化 / 状态</td><td>POST/GET</td><td><code class="inline">/admin/telegram/init</code>、<code class="inline">/admin/telegram/status</code></td></tr>
<tr><td>机器人设置</td><td>GET/POST</td><td><code class="inline">/admin/telegram/settings</code></td></tr>
<tr><td>MiniApp 接口</td><td>POST</td><td><code class="inline">/telegram/get_bind_address|new_address|bind_address|unbind_address|get_mail</code>（需 initData）</td></tr>
</table>

<h2 id="agent">Agent 接入（AI 助手）</h2>
<p>面向 Claude / Codex / Cursor 等 Agent 消费临时邮箱的标准流程：</p>
<ol>
<li>用户在浏览器创建/登录地址，复制 <b>Address JWT</b> 与 <b>API 地址</b>（同源）。</li>
<li>Agent 用 <code class="inline">Authorization: Bearer $JWT</code> 调 <code class="inline">/api/settings</code> 自检。</li>
<li>轮询 <code class="inline">/api/parsed_mails</code>，按 <code class="inline">id</code> 去重，初始 3s、退避封顶 10s。</li>
<li>验证码/链接直接读 parsed 的 <code class="inline">text</code>/<code class="inline">html</code>；需要原始字节时退回 <code class="inline">/api/mail/:id</code>。</li>
</ol>
<div class="note"><b>轮询纪律：</b>不要快于每秒 1 次；遇 <code class="inline">429</code> 退避重试；不要混用 Address JWT 与 User JWT。</div>

<h2 id="err">错误与注意事项</h2>
<table><tr><th>错误</th><th>含义</th></tr>
<tr><td>401 Invalid address credential</td><td>Address JWT 错误/过期，或 header 填错位置</td></tr>
<tr><td>401 Need admin password</td><td>缺少/错误 <code class="inline">x-admin-auth</code></td></tr>
<tr><td>400 Invalid limit / offset</td><td>limit 必须在 1..100，offset ≥ 0</td></tr>
<tr><td>429 Rate limit exceeded</td><td>触发限流，请退避</td></tr>
<tr><td>403 Access blocked</td><td>命中 IP 黑名单/白名单</td></tr>
</table>
<p class="note" style="margin-top:24px">本页为静态说明；接口以实际部署为准。若部署开启了 Turnstile，<code class="inline">/api/new_address</code> 等需额外携带 <code class="inline">cf_token</code>。</p>
</div></body></html>
`

func (a *App) apiDocs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write([]byte(apiDocsHTML))
}
