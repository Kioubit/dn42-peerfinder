import {isRecord, isString, isNumber, isBoolean, isNullable} from "@/types/main.ts";

export type AgentInfo = {
    uuid: string;
    id: string;
    endpoint: string;
    hmac_key: string;
    version: string | null;
    added_at: Date | null;
    last_seen: Date | null;
    last_probed: Date | null;
    disabled: boolean;
};

export type AgentStatistics = {
    active: number;
    registered: number;
    update_time: Date;
};

export type RegisterAgentResponse = {
    endpoint: string;
    hmac_key: string;
};

export type EditAgentPayload = {
    name: string;
    endpoint: string;
    disabled: boolean;
};

//----------------------------------------------------------------------------------------------------------------------

export function parseAgentInfo(data: unknown): AgentInfo {
    if (
        !isRecord(data) ||
        !isString(data.uuid) ||
        !isString(data.id) ||
        !isString(data.endpoint) ||
        !isString(data.hmac_key) ||
        !isNullable(isString)(data.version) ||
        !isBoolean(data.disabled)
    ) {
        throw new Error("Invalid agent info");
    }

    const agent = data as AgentInfo;

    return {
        ...agent,
        added_at: isString(agent.added_at) ? new Date(agent.added_at) : null,
        last_seen: isString(agent.last_seen) ? new Date(agent.last_seen) : null,
        last_probed: isString(agent.last_probed) ? new Date(agent.last_probed) : null,
    };
}

export function parseAgentStatistics(data: unknown): AgentStatistics {
    if (
        !isRecord(data) ||
        !isNumber(data.active) ||
        !isNumber(data.registered) ||
        !isString(data.update_time)
    ) {
        throw new Error("Invalid agent statistics");
    }

    const statistics = data as AgentStatistics;

    return {
        active: statistics.active,
        registered: statistics.registered,
        update_time: new Date(statistics.update_time),
    };
}

export function isRegisterAgentResponse(data: unknown): data is RegisterAgentResponse {
    return (
        isRecord(data) &&
        isString(data.endpoint) &&
        isString(data.hmac_key)
    );
}