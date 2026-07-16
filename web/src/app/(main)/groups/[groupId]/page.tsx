"use client"

import { useParams } from "next/navigation"
import { GroupDetail } from "@/features/groups/components/GroupDetail"

export default function GroupDetailPage() {
  const params = useParams<{ groupId: string }>()
  return <GroupDetail groupId={params.groupId} />
}
