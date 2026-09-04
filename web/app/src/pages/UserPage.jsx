import { useState } from 'react'
import { Users } from 'lucide-react'
import Layout from '../components/Layout'
import UsersPage from './admin/UserPage'
import { useApp } from '../context/AppContext'

export default function UserPage() {
  const { t } = useApp()
  return <Layout><div className="mx-auto max-w-[1400px] px-4 py-5 sm:px-6">
    <div className="mb-5"><h1 className="text-2xl font-semibold">用户管理</h1><p className="mt-1 text-sm text-muted-foreground">创建用户并直接配置邮箱、邮件和发信额度</p></div>
    <UsersPage t={t} />
  </div></Layout>
}
