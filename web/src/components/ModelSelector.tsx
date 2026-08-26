// web/src/components/ModelSelector.tsx
"use client";

import { useEffect, useState, useCallback } from "react";

interface AvailableModel {
  id: string;
  name: string;
  provider: string;
  sizeGB?: string;
  isActive: boolean;
}

interface ModelsResponse {
  provider: string;
  currentModel: string;
  models: AvailableModel[];
  note?: string;
  error?: string;
}

export default function ModelSelector() {
  const [data, setData] = useState<ModelsResponse | null>(null);
  const [currentModel, setCurrentModel] = useState<string>("");
  const [loading, setLoading] = useState(true);
  const [switching, setSwitching] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const gatewayURL = process.env.NEXT_PUBLIC_GATEWAY_URL ?? "http://localhost:8080";

  const fetchModels = useCallback(async () => {
    try {
      const res = await fetch(`${gatewayURL}/api/models`);
      const json: ModelsResponse = await res.json();
      setData(json);
      setCurrentModel(json.currentModel ?? "");
      setError(json.error ?? null);
    } catch {
      setError("Cannot reach gateway. Is it running?");
    } finally {
      setLoading(false);
    }
  }, [gatewayURL]);

  useEffect(() => {
    fetchModels();
  }, [fetchModels]);

  const handleSwitch = async (modelId: string) => {
    if (modelId === currentModel) return;
    setSwitching(true);
    try {
      const res = await fetch(`${gatewayURL}/api/models/switch`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ modelId }),
      });
      if (res.ok) {
        setCurrentModel(modelId);
      } else {
        const err = await res.json();
        setError(err.error ?? "Failed to switch model");
      }
    } catch {
      setError("Network error when switching model");
    } finally {
      setSwitching(false);
    }
  };

  // Don't render anything while loading or if only one model (nothing to choose)
  if (loading) return null;
  if (!data || (data.models?.length ?? 0) <= 1) return null;

  return (
    <div className="model-selector rounded-lg border border-gray-200 bg-gray-50 p-3 text-sm">
      <div className="flex items-center gap-2">
        <label
          htmlFor="model-select"
          className="font-medium text-gray-600 whitespace-nowrap"
        >
          AI Model
          <span className="ml-1 text-xs text-gray-400">
            ({data.provider})
          </span>
        </label>

        <select
          id="model-select"
          value={currentModel}
          disabled={switching}
          onChange={(e) => handleSwitch(e.target.value)}
          className="flex-1 rounded border border-gray-300 bg-white px-2 py-1
                     text-sm focus:outline-none focus:ring-2 focus:ring-blue-400
                     disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {data.models.map((m) => (
            <option key={m.id} value={m.id}>
              {m.name}
              {m.sizeGB ? ` — ${m.sizeGB}` : ""}
            </option>
          ))}
        </select>

        {switching && (
          <span className="text-xs text-blue-500 animate-pulse">
            Switching…
          </span>
        )}
      </div>

      {error && (
        <p className="mt-1 text-xs text-red-500">{error}</p>
      )}

      <p className="mt-1 text-xs text-gray-400">
        Session only — update <code>OLLAMA_MODEL</code> in .env to persist
      </p>
    </div>
  );
}