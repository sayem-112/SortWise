export const statuses: Record<string, { label: string; tone: string }> = {
  pending: { label: "Queued", tone: "gray" },
  processing: { label: "Analyzing", tone: "blue" },
  completed: { label: "Organized", tone: "green" },
  failed: { label: "Needs attention", tone: "red" },
  blocked: { label: "AI not configured", tone: "orange" },
};

export const statusOptions = Object.entries(statuses).map(([value, { label, tone }]) => ({ value, label, tone }));

