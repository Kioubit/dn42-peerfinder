import {isArrayOf, isNullable, isRecord, isOptional, isString, isNumber} from "@/types/main.ts";

export type NetworkList = {
    items: ListResponseNetwork[];
    total: number;
};

export type Network = {
    Name: string;
    Mnt: string;
    Description: string;
    URL: string;
    Tags: readonly Tag[] | null;
    Servers?: readonly Server[];
};

export type ListResponseNetwork = Network & {
    asn: string;
    serverCount: number;
};

export type Server = {
    ID: string;
    Address: string;
    CountryCode: string;
    City: string;
    Tags: readonly Tag[] | null;
};

export type Tag = KnownTag | (string & {});

export type KnownTag = typeof knownTags[number];
export const knownTags = [
    'automated-peering', 'semi-automated-peering', 'fast-reply', 'testing',
    'e-mail', 'irc', 'telegram', 'matrix', 'xmpp', 'signal', 'wireguard',
    'openvpn', 'gre', 'ipsec', 'tinc', 'fastd', 'stunnel', 'v4-only',
    'v6-only', 'NAT', 'mp-bgp', 'enh', 'bfd', 'contact-required', 'dynamic-ip'
] as const;

export type AvailableCountriesResponse = {
    countries: string[];
};

//----------------------------------------------------------------------------------------------------------------------

export function isNetworkList(value: unknown): value is NetworkList {
    return (
        isRecord(value) &&
        isArrayOf<ListResponseNetwork>(isListResponseNetwork)(value.items) &&
        isNumber(value.total)
    );
}


export function isListResponseNetwork(value: unknown): value is ListResponseNetwork {
    return (
        isRecord(value) &&
        isNetwork(value) &&
        isString((value as ListResponseNetwork).asn) &&
        isNumber((value as ListResponseNetwork).serverCount)
    );
}

export function isNetwork(value: unknown): value is Network {
    if (!isRecord(value)) {
        return false;
    }

    const network = value as Network;

    return (
        isString(network.Name) &&
        isString(network.Mnt) &&
        isString(network.Description) &&
        isString(network.URL) &&
        isNullable(isArrayOf<Tag>(isTag))(network.Tags) &&
        isServerList(network.Servers)
    );
}

export function isTag(value: unknown): value is Tag {
    return isString(value);
}

export function isServerList(value: unknown): value is Server[] | undefined {
    return isOptional(isArrayOf<Server>(isServer))(value);
}

export function isServer(value: unknown): value is Server {
    if (!isRecord(value)) {
        return false;
    }

    return (
        isString(value.ID) &&
        isString(value.Address) &&
        isString(value.CountryCode) &&
        isString(value.City) &&
        isNullable(isArrayOf<Tag>(isTag))(value.Tags)
    );
}

export function isAvailableCountriesResponse(value: unknown): value is AvailableCountriesResponse {
    if (!isRecord(value)) {
        return false;
    }
    return isArrayOf<string>(isString)(value.countries);
}