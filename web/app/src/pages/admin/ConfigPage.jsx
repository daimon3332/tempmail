import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button, Card, Input } from '../../components/ui'
import api from '../../lib/api'

// Config page: hot-reloadable settings (runtime_config). Saving applies immediately.
export default function ConfigPage({ t }) {
  const q = useQuery({ queryKey: ['runtime_config'], queryFn: () => api('/admin/runtime_config').then(r => r.data) })
  const [f, setF] = useState(null)
  const [saved, setSaved] = useState(false)
  if (q.data && f === null) setF(q.data)
  const up = (k, v) => setF(x => ({ ...x, [k]: v }))
  const state = f || q.data || {}
  const save = async () => {
    const r = await api('/admin/runtime_config', 'POST', state)
    if (r.status === 200) { setSaved(true); q.refetch(); setTimeout(() => setSaved(false), 2000) }
    else alert(r.data || '保存失败')
  }
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">配置</h2>
        <div className="flex items-center gap-2">
          {saved && <span className="text-sm text-emerald-500">✓ 已保存并即时生效</span>}
          <Button size="sm" onClick={save}>{t('save')}</Button>
        </div>
      </div>

      <Section title="站点信息">
        <Field label="标题" value={state.title} onChange={v => up('title', v)} />
        <Field label="公告" value={state.announcement} onChange={v => up('announcement', v)} />
        <Field label="版权" value={state.copyright} onChange={v => up('copyright', v)} />
        <Field label="联系邮箱" value={state.admin_contact} onChange={v => up('admin_contact', v)} />
        <Field label="默认语言" value={state.default_lang} onChange={v => up('default_lang', v)} />
      </Section>

      <Section title="认证与 API Key">
        <Field label="站点访问密码（启用 x-custom-auth）" type="password" value={state.site_password} onChange={v => up('site_password', v)} placeholder="留空不启用" />
        <Field label="管理员密码（启用 x-admin-auth 覆盖）" type="password" value={state.admin_password} onChange={v => up('admin_password', v)} placeholder="留空用 .env" />
        <Field label="API Key（第三方调用 /api/*）" value={state.api_key} onChange={v => up('api_key', v)} placeholder="留空关闭" />
      </Section>

      <Section title="地址创建">
        <Field label="前缀" value={state.prefix} onChange={v => up('prefix', v)} />
        <div className="grid grid-cols-2 gap-3">
          <Field label="最小长度" type="number" value={state.min_address_len} onChange={v => up('min_address_len', parseInt(v) || 1)} />
          <Field label="最大长度" type="number" value={state.max_address_len} onChange={v => up('max_address_len', parseInt(v) || 30)} />
        </div>
        <Field label="允许创建邮箱" type="checkbox" value={state.enable_user_create_email} onChange={v => up('enable_user_create_email', !!v)} />
        <Field label="禁止匿名创建" type="checkbox" value={state.disable_anonymous_user_create_email} onChange={v => up('disable_anonymous_user_create_email', !!v)} />
        <Field label="禁止自定义邮箱名" type="checkbox" value={state.disable_custom_address_name} onChange={v => up('disable_custom_address_name', !!v)} />
      </Section>

      <Section title="功能开关">
        <ToggleRow label="允许删除邮件" value={state.enable_user_delete_email} onChange={v => up('enable_user_delete_email', !!v)} />
        <ToggleRow label="已读状态" value={state.enable_mail_read_status} onChange={v => up('enable_mail_read_status', !!v)} />
        <ToggleRow label="地址密码登录" value={state.enable_address_password} onChange={v => up('enable_address_password', !!v)} />
        <ToggleRow label="Webhook" value={state.enable_webhook} onChange={v => up('enable_webhook', !!v)} />
        <ToggleRow label="自动回复" value={state.enable_auto_reply} onChange={v => up('enable_auto_reply', !!v)} />
        <ToggleRow label="垃圾邮件检查" value={state.enable_check_junk_mail} onChange={v => up('enable_check_junk_mail', !!v)} />
        <ToggleRow label="拒绝未知地址" value={state.block_unknown_address} onChange={v => up('block_unknown_address', !!v)} />
      </Section>

      <Section title="AI 提取（验证码/链接自动提取）">
        <ToggleRow label="启用 AI 提取" value={state.ai_enabled} onChange={v => up('ai_enabled', !!v)} />
        <Field label="AI 接口地址（OpenAI 兼容 /v1/chat/completions）" value={state.ai_endpoint} onChange={v => up('ai_endpoint', v)} placeholder="https://api.openai.com/v1" />
        <Field label="AI API Key" type="password" value={state.ai_api_key} onChange={v => up('ai_api_key', v)} />
        <Field label="模型" value={state.ai_model} onChange={v => up('ai_model', v)} placeholder="gpt-4o-mini" />
        <ToggleRow label="仅提取白名单地址" value={state.ai_enable_allow_list} onChange={v => up('ai_enable_allow_list', !!v)} />
        <Field label="白名单地址（逗号分隔）" value={(state.ai_allow_list || []).join(',')} onChange={v => up('ai_allow_list', v.split(',').map(s => s.trim()).filter(Boolean))} />
      </Section>

      <p className="text-xs text-muted-foreground">提示：域名列表、SMTP 中继、S3 等需改 .env 并重启容器生效。其余保存即生效。</p>
    </div>
  )
}

function Section({ title, children }) {
  return <Card className="p-4"><h3 className="mb-3 text-sm font-semibold text-muted-foreground">{title}</h3><div className="space-y-3">{children}</div></Card>
}
function Field({ label, value, onChange, type = 'text', placeholder }) {
  return <div className="flex items-center gap-3">
    <label className="w-40 shrink-0 text-sm">{label}</label>
    <Input type={type} value={value ?? ''} placeholder={placeholder} onChange={e => onChange(e.target.value)} className="flex-1" />
  </div>
}
function ToggleRow({ label, value, onChange }) {
  return <div className="flex items-center gap-3"><label className="w-40 shrink-0 text-sm">{label}</label><input type="checkbox" checked={!!value} onChange={e => onChange(e.target.checked)} /></div>
}
