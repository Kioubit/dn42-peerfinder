export const UI_PAGES = ["main", "edit", "discover"] as const;

export type UIPage = typeof UI_PAGES[number];

export function isUIPage(value: string): value is UIPage {
    return (UI_PAGES as readonly string[]).includes(value);
}

//----------------------------------------------------------------------------------------------------------------------

export function isUnknownArray(value: unknown): value is unknown[] {
    return Array.isArray(value);
}

export function isRecord(value: unknown): value is Record<string, unknown> {
    return typeof value === "object" && value !== null && !Array.isArray(value);
}

type Validator<T> = (v: unknown) => v is T;
export function isArrayOf<T>(check: Validator<T>): Validator<T[]> {
    return (v): v is T[] => isUnknownArray(v) && v.every(check);
}
export function isRecordWithValuesOf<T>(check: Validator<T>): Validator<Record<string, T>> {
    return (v): v is Record<string, T> => isRecord(v) && Object.values(v).every(check);
}
export function isOptional<T>(check: Validator<T>): Validator<T | undefined> {
    return (v): v is T | undefined => v === undefined || check(v);
}
export function isNullable<T>(check: Validator<T>): Validator<T | null> {
    return (v): v is T | null => v === null || check(v);
}

export const isBoolean = (v:unknown) => typeof v === "boolean";
export const isNumber= (v:unknown) => typeof v === "number";
export const isString= (v:unknown) => typeof v === "string";
