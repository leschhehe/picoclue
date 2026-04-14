import {
  IconFlask,
  IconPlay,
  IconPause,
  IconStop,
  IconFileText,
  IconDownload,
  IconRefresh,
  IconCheck,
  IconX,
  IconLoader,
} from "@tabler/icons-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { cn } from "@/lib/utils"

export type ResearchMode = "quick" | "standard" | "deep"
export type ResearchStatus =
  | "idle"
  | "planning"
  | "executing"
  | "summarizing"
  | "synthesizing"
  | "completed"
  | "failed"
  | "paused"

export interface ResearchTask {
  id: string
  description: string
  status: "pending" | "running" | "completed" | "failed"
  result?: string
  error?: string
}

export interface ResearchSession {
  id: string
  goal: string
  mode: ResearchMode
  status: ResearchStatus
  tasks: ResearchTask[]
  createdAt: string
  updatedAt?: string
  reportPath?: string
}

interface ResearchPanelProps {
  currentSession?: ResearchSession
  onStartResearch: (goal: string, mode: ResearchMode) => void
  onResumeResearch: (sessionId: string) => void
  onCancelResearch: (sessionId: string) => void
}

export function ResearchPanel({
  currentSession,
  onStartResearch,
  onResumeResearch,
  onCancelResearch,
}: ResearchPanelProps) {
  const { t } = useTranslation()
  const [isDialogOpen, setIsDialogOpen] = useState(false)
  const [researchGoal, setResearchGoal] = useState("")
  const [selectedMode, setSelectedMode] = useState<ResearchMode>("standard")

  const handleStartResearch = () => {
    if (!researchGoal.trim()) {
      toast.error("Please enter a research goal")
      return
    }

    onStartResearch(researchGoal.trim(), selectedMode)
    setIsDialogOpen(false)
    setResearchGoal("")

    toast.success(
      t("chat.research.notifications.started", {
        goal: researchGoal.trim(),
        mode: t(`chat.research.modes.${selectedMode}.label`),
      }),
    )
  }

  const handleCancel = () => {
    if (currentSession) {
      onCancelResearch(currentSession.id)
      toast.info("Research cancelled")
    }
  }

  const getStatusIcon = (status: ResearchStatus) => {
    switch (status) {
      case "planning":
        return <IconLoader className="h-4 w-4 animate-spin" />
      case "executing":
        return <IconLoader className="h-4 w-4 animate-spin" />
      case "summarizing":
        return <IconFileText className="h-4 w-4" />
      case "synthesizing":
        return <IconFlask className="h-4 w-4" />
      case "completed":
        return <IconCheck className="h-4 w-4 text-green-500" />
      case "failed":
        return <IconX className="h-4 w-4 text-red-500" />
      case "paused":
        return <IconPause className="h-4 w-4 text-yellow-500" />
      default:
        return null
    }
  }

  const getStatusLabel = (status: ResearchStatus, session?: ResearchSession) => {
    if (status === "executing" && session) {
      const completed = session.tasks.filter((t) => t.status === "completed").length
      return t("chat.research.status.executing", {
        current: completed + 1,
        total: session.tasks.length,
      })
    }
    return t(`chat.research.status.${status}`)
  }

  const getProgressPercentage = (session?: ResearchSession) => {
    if (!session || session.tasks.length === 0) return 0
    const completed = session.tasks.filter((t) => t.status === "completed").length
    return Math.round((completed / session.tasks.length) * 100)
  }

  const isActive = currentSession && 
    ["planning", "executing", "summarizing", "synthesizing"].includes(currentSession.status)

  return (
    <>
      <div className="border-border/40 bg-card/50 mx-4 mb-4 rounded-xl border">
        <div className="p-4">
          <div className="mb-3 flex items-center justify-between">
            <div className="flex items-center gap-2">
              <IconFlask className="text-primary h-5 w-5" />
              <div>
                <h3 className="font-medium">{t("chat.research.title")}</h3>
                <p className="text-muted-foreground text-xs">
                  {t("chat.research.description")}
                </p>
              </div>
            </div>

            {!currentSession || currentSession.status === "idle" ? (
              <Button
                size="sm"
                onClick={() => setIsDialogOpen(true)}
                className="gap-2"
              >
                <IconPlay className="h-4 w-4" />
                {t("chat.research.startButton")}
              </Button>
            ) : (
              <div className="flex items-center gap-2">
                {isActive && (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => onResumeResearch(currentSession.id)}
                    className="gap-2"
                  >
                    <IconPause className="h-4 w-4" />
                    {t("chat.research.actions.resume")}
                  </Button>
                )}
                <Button
                  variant="destructive"
                  size="sm"
                  onClick={handleCancel}
                  className="gap-2"
                >
                  <IconStop className="h-4 w-4" />
                  {t("chat.research.actions.cancel")}
                </Button>
              </div>
            )}
          </div>

          {currentSession && currentSession.status !== "idle" && (
            <div className="space-y-3">
              <div className="flex items-center justify-between text-sm">
                <span className="text-muted-foreground">
                  {getStatusLabel(currentSession.status, currentSession)}
                </span>
                <span className="text-muted-foreground">
                  {t("chat.research.progress.tasksCompleted", {
                    completed: currentSession.tasks.filter(
                      (t) => t.status === "completed",
                    ).length,
                    total: currentSession.tasks.length,
                  })}
                </span>
              </div>

              <div className="bg-muted h-2 overflow-hidden rounded-full">
                <div
                  className={cn(
                    "h-full transition-all duration-500",
                    currentSession.status === "failed"
                      ? "bg-red-500"
                      : currentSession.status === "completed"
                        ? "bg-green-500"
                        : "bg-primary",
                  )}
                  style={{ width: `${getProgressPercentage(currentSession)}%` }}
                />
              </div>

              {currentSession.status === "completed" && currentSession.reportPath && (
                <div className="flex items-center gap-2">
                  <Button variant="outline" size="sm" className="gap-2">
                    <IconFileText className="h-4 w-4" />
                    {t("chat.research.actions.viewResults")}
                  </Button>
                  <Button variant="outline" size="sm" className="gap-2">
                    <IconDownload className="h-4 w-4" />
                    {t("chat.research.actions.downloadReport")}
                  </Button>
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => setIsDialogOpen(true)}
                    className="gap-2"
                  >
                    <IconFlask className="h-4 w-4" />
                    {t("chat.research.actions.newResearch")}
                  </Button>
                </div>
              )}
            </div>
          )}
        </div>
      </div>

      <Dialog open={isDialogOpen} onOpenChange={setIsDialogOpen}>
        <DialogContent className="sm:max-w-[500px]">
          <DialogHeader>
            <DialogTitle>{t("chat.research.title")}</DialogTitle>
            <DialogDescription>
              {t("chat.research.description")}
            </DialogDescription>
          </DialogHeader>

          <div className="grid gap-4 py-4">
            <div className="grid gap-2">
              <Label htmlFor="research-goal">
                Research Goal
              </Label>
              <Input
                id="research-goal"
                placeholder="e.g., Analyze the market for AI-powered medical diagnostics"
                value={researchGoal}
                onChange={(e) => setResearchGoal(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    handleStartResearch()
                  }
                }}
              />
            </div>

            <div className="grid gap-2">
              <Label>Research Depth</Label>
              <Select
                value={selectedMode}
                onValueChange={(value: ResearchMode) => setSelectedMode(value)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="quick">
                    <div className="flex items-center justify-between w-full">
                      <span>{t("chat.research.modes.quick.label")}</span>
                      <span className="text-muted-foreground text-xs">
                        {t("chat.research.modes.quick.description")}
                      </span>
                    </div>
                  </SelectItem>
                  <SelectItem value="standard">
                    <div className="flex items-center justify-between w-full">
                      <span>{t("chat.research.modes.standard.label")}</span>
                      <span className="text-muted-foreground text-xs">
                        {t("chat.research.modes.standard.description")}
                      </span>
                    </div>
                  </SelectItem>
                  <SelectItem value="deep">
                    <div className="flex items-center justify-between w-full">
                      <span>{t("chat.research.modes.deep.label")}</span>
                      <span className="text-muted-foreground text-xs">
                        {t("chat.research.modes.deep.description")}
                      </span>
                    </div>
                  </SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setIsDialogOpen(false)}>
              {t("common.cancel")}
            </Button>
            <Button onClick={handleStartResearch} className="gap-2">
              <IconPlay className="h-4 w-4" />
              {t("chat.research.startButton")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
