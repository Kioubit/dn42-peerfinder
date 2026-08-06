import {isBoolean, isNumber, isOptional, isRecord, isRecordWithValuesOf, isString} from "@/types/main.ts";

export type PingResult = {
    asn: string;
    node?: string;
    latency?: number;
    jitter?: number;
    min_rtt?: number;
    max_rtt?: number;
    sent: number;
    recv: number;
    reachable: boolean;
};

export type PingStartResponse = {
    total: number;
};

export type PingErrorResponse = {
    message?: string;
};

export type PingMeta = Record<string, PingMetaNetwork>;

export type PingMetaNetwork = {
    network: string;
    description: string;
    servers: Record<string, PingServerInfo>;
};

export type PingServerInfo = {
    CountryCode: string;
    City: string;
}

//----------------------------------------------------------------------------------------------------------------------

export function isPingResult(value: unknown): value is PingResult {
    if (!isRecord(value)) {
        return false;
    }

    return (
        isString(value.asn) &&
        isOptional(isString)(value.node) &&
        isOptional(isNumber)(value.latency) &&
        isOptional(isNumber)(value.jitter) &&
        isOptional(isNumber)(value.min_rtt) &&
        isOptional(isNumber)(value.max_rtt) &&
        isNumber(value.sent) &&
        isNumber(value.recv) &&
        isBoolean(value.reachable)
    );
}

export function isPingErrorResponse(value: unknown): value is PingErrorResponse {
    return isRecord(value) && isOptional(isString)(value.message);
}

export function isPingStartResponse(value: unknown): value is PingStartResponse {
    return isRecord(value) && isNumber(value.total);
}

export function isPingMeta(value: unknown): value is PingMeta {
    return isRecordWithValuesOf(isPingMetaNetwork)(value);
}

function isPingMetaNetwork(value: unknown): value is PingMetaNetwork {
    if (!isRecord(value)) {
        return false;
    }

    return (
        isString(value.network) &&
        isString(value.description) &&
        isRecordWithValuesOf(isPingServerInfo)(value.servers)
    );
}

function isPingServerInfo(value: unknown): value is PingServerInfo {
    if (!isRecord(value)) {
        return false;
    }

    return (
        isString(value.CountryCode) &&
        isString(value.City)
    );
}