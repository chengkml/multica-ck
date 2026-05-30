"use client";

import { useEffect, useState } from "react";
import { Save, Plus, Trash2, Pencil, X } from "lucide-react";
import { Input } from "@multica/ui/components/ui/input";
import { Button } from "@multica/ui/components/ui/button";
import { Card, CardContent } from "@multica/ui/components/ui/card";
import { toast } from "sonner";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuthStore } from "@multica/core/auth";
import { useWorkspaceId } from "@multica/core/hooks";
import { useCurrentWorkspace } from "@multica/core/paths";
import { memberListOptions, workspaceKeys } from "@multica/core/workspace/queries";
import { api } from "@multica/core/api";
import type { Workspace, WorkspaceRepo } from "@multica/core/types";
import { useT } from "../../i18n";

function dropAndShiftIndex(set: Set<number>, removed: number): Set<number> {
  const next = new Set<number>();
  set.forEach((i) => {
    if (i === removed) return;
    next.add(i > removed ? i - 1 : i);
  });
  return next;
}

function normalizeDefaultBranch(branch: string | undefined): string {
  return (branch ?? "").trim();
}

function isDirty(local: WorkspaceRepo[], saved: WorkspaceRepo[]): boolean {
  if (local.length !== saved.length) return true;
  return local.some((r, i) => {
    if (r.url !== saved[i]?.url) return true;
    return normalizeDefaultBranch(r.default_branch) !== normalizeDefaultBranch(saved[i]?.default_branch);
  });
}

export function RepositoriesTab() {
  const { t } = useT("settings");
  const user = useAuthStore((s) => s.user);
  const workspace = useCurrentWorkspace();
  const wsId = useWorkspaceId();
  const qc = useQueryClient();
  const { data: members = [] } = useQuery(memberListOptions(wsId));

  const [repos, setRepos] = useState<WorkspaceRepo[]>(workspace?.repos ?? []);
  const [editingIndices, setEditingIndices] = useState<Set<number>>(new Set());
  const [saving, setSaving] = useState(false);

  const currentMember = members.find((m) => m.user_id === user?.id) ?? null;
  const canManageWorkspace = currentMember?.role === "owner" || currentMember?.role === "admin";

  useEffect(() => {
    setRepos(workspace?.repos ?? []);
  }, [workspace]);

  const savedRepos = workspace?.repos ?? [];
  const dirty = isDirty(repos, savedRepos);

  const handleSave = async () => {
    if (!workspace) return;
    setSaving(true);
    try {
      const updated = await api.updateWorkspace(workspace.id, { repos });
      qc.setQueryData(workspaceKeys.list(), (old: Workspace[] | undefined) =>
        old?.map((ws) => (ws.id === updated.id ? updated : ws)),
      );
      setEditingIndices(new Set());
      toast.success(t(($) => $.repositories.toast_saved));
    } catch (e) {
      toast.error(e instanceof Error ? e.message : t(($) => $.repositories.toast_save_failed));
    } finally {
      setSaving(false);
    }
  };

  const handleAddRepo = () => {
    const nextIndex = repos.length;
    setRepos([...repos, { url: "", default_branch: "" }]);
    setEditingIndices(new Set(editingIndices).add(nextIndex));
  };

  const handleRemoveRepo = (index: number) => {
    setRepos(repos.filter((_, i) => i !== index));
    setEditingIndices(dropAndShiftIndex(editingIndices, index));
  };

  const handleRepoChange = (index: number, value: string) => {
    setRepos(repos.map((r, i) => (i === index ? { ...r, url: value } : r)));
  };

  const handleDefaultBranchChange = (index: number, value: string) => {
    setRepos(repos.map((r, i) => (i === index ? { ...r, default_branch: value } : r)));
  };

  const handleEditRepo = (index: number) => {
    setEditingIndices(new Set(editingIndices).add(index));
  };

  const handleCancelEdit = (index: number) => {
    const savedRepo = savedRepos[index];
    if (!savedRepo) {
      // Newly added row that was never persisted — drop it entirely.
      handleRemoveRepo(index);
      return;
    }
    setRepos(
      repos.map((r, i) =>
        i === index
          ? {
              ...r,
              url: savedRepo.url,
              default_branch: savedRepo.default_branch ?? "",
            }
          : r,
      ),
    );
    const next = new Set(editingIndices);
    next.delete(index);
    setEditingIndices(next);
  };

  if (!workspace) return null;

  return (
    <div className="space-y-8">
      <section className="space-y-4">
        <h2 className="text-sm font-semibold">{t(($) => $.repositories.section_title)}</h2>

        <Card>
          <CardContent className="space-y-3">
            <p className="text-xs text-muted-foreground">
              {t(($) => $.repositories.description)}
            </p>

            {repos.length === 0 && (
              <p className="text-xs text-muted-foreground italic">
                {t(($) => $.repositories.empty)}
              </p>
            )}

            {repos.map((repo, index) => {
              const isEditing = editingIndices.has(index);
              return (
                <div
                  key={index}
                  className="group flex items-start gap-2"
                >
                  {isEditing ? (
                    <div className="flex-1 min-w-0 space-y-2">
                      <Input
                        type="text"
                        value={repo.url}
                        onChange={(e) => handleRepoChange(index, e.target.value)}
                        disabled={!canManageWorkspace}
                        placeholder={t(($) => $.repositories.url_placeholder)}
                        aria-label={t(($) => $.repositories.url_label)}
                        className="text-sm"
                      />
                      <Input
                        type="text"
                        value={repo.default_branch ?? ""}
                        onChange={(e) => handleDefaultBranchChange(index, e.target.value)}
                        disabled={!canManageWorkspace}
                        placeholder={t(($) => $.repositories.default_branch_placeholder)}
                        aria-label={t(($) => $.repositories.default_branch_label)}
                        className="text-sm"
                      />
                    </div>
                  ) : (
                    <div
                      className="flex-1 min-w-0 rounded-md border bg-muted/50 px-3 py-2"
                      title={repo.url}
                    >
                      <p className="truncate font-mono text-xs text-muted-foreground">
                        {repo.url || t(($) => $.repositories.url_empty)}
                      </p>
                      <p className="mt-1 text-[11px] text-muted-foreground">
                        {t(($) => $.repositories.default_branch_prefix)}{" "}
                        <span className="font-mono">
                          {normalizeDefaultBranch(repo.default_branch) ||
                            t(($) => $.repositories.default_branch_empty)}
                        </span>
                      </p>
                    </div>
                  )}
                  {canManageWorkspace && (
                    <div
                      className={
                        isEditing
                          ? "flex shrink-0 items-center gap-0.5"
                          : "flex shrink-0 items-center gap-0.5 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100 [@media(hover:none)]:opacity-100"
                      }
                    >
                      {!isEditing && (
                        <Button
                          variant="ghost"
                          size="icon"
                          aria-label={t(($) => $.repositories.edit_aria)}
                          className="text-muted-foreground hover:text-foreground"
                          onClick={() => handleEditRepo(index)}
                        >
                          <Pencil className="h-3.5 w-3.5" />
                        </Button>
                      )}
                      {isEditing && (
                        <Button
                          variant="ghost"
                          size="icon"
                          aria-label={t(($) => $.repositories.cancel_aria)}
                          className="text-muted-foreground hover:text-foreground"
                          onClick={() => handleCancelEdit(index)}
                        >
                          <X className="h-3.5 w-3.5" />
                        </Button>
                      )}
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label={t(($) => $.repositories.delete_aria)}
                        className="text-muted-foreground hover:text-destructive"
                        onClick={() => handleRemoveRepo(index)}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  )}
                </div>
              );
            })}

            {canManageWorkspace && (
              <div className="flex flex-wrap items-center justify-between gap-2 pt-1">
                <Button variant="outline" size="sm" onClick={handleAddRepo}>
                  <Plus className="h-3 w-3" />
                  {t(($) => $.repositories.add)}
                </Button>
                <div className="flex items-center gap-3">
                  {!dirty && repos.length > 0 && (
                    <span className="text-xs text-muted-foreground">
                      {t(($) => $.repositories.saved_hint)}
                    </span>
                  )}
                  <Button
                    size="sm"
                    onClick={handleSave}
                    disabled={saving || !dirty}
                  >
                    <Save className="h-3 w-3" />
                    {saving ? t(($) => $.repositories.saving) : t(($) => $.repositories.save)}
                  </Button>
                </div>
              </div>
            )}

            {!canManageWorkspace && (
              <p className="text-xs text-muted-foreground">
                {t(($) => $.repositories.manage_hint)}
              </p>
            )}
          </CardContent>
        </Card>
      </section>
    </div>
  );
}
