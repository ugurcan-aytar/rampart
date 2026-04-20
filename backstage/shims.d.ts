// Ambient type declarations for Backstage / MUI / Emotion / Express /
// react-router-dom. Full types come from installed packages; these stubs
// exist so `yarn typecheck` succeeds in Adım 5's code-only state,
// before the full Backstage toolchain installs in Adım 7.
//
// Signatures are loose but generic-friendly — enough to let the plugin
// source compile cleanly. Once real packages are installed their types
// take precedence.

declare module '@backstage/core-plugin-api' {
  export interface ApiRef<T> {
    readonly id: string;
    // Phantom field so T is preserved through useApi inference.
    readonly __apiType?: T;
  }
  export function createApiRef<T>(opts: { id: string }): ApiRef<T>;
  export function createPlugin(opts: any): any;
  export function createRouteRef(opts: any): any;
  export function createApiFactory<T = any>(opts: any): any;
  export function createRoutableExtension<T = any>(opts: any): any;
  export function useApi<T>(ref: ApiRef<T>): T;
  export const configApiRef: any;
}

declare module '@backstage/core-components' {
  import type { ReactNode } from 'react';
  export const Page: React.ComponentType<{ themeId?: string; children?: ReactNode }>;
  export const Header: React.ComponentType<{ title?: string; subtitle?: string; children?: ReactNode }>;
  export const Content: React.ComponentType<{ children?: ReactNode }>;
  export const Table: React.ComponentType<any>;
  export const InfoCard: React.ComponentType<{ title?: string; children?: ReactNode }>;
  export const Progress: React.ComponentType<any>;
  export const Link: React.ComponentType<{ to: string; children?: ReactNode }>;
}

declare module '@backstage/theme' {
  export const lightTheme: any;
  export const darkTheme: any;
}

declare module '@backstage/plugin-catalog-react' {
  export function useEntity<T = any>(): { entity: T };
}

declare module '@backstage/plugin-scaffolder-node' {
  export type ActionContext<Input = any, Output = any> = {
    input: Input;
    output: (name: string, value: unknown) => void;
    logger: { info(msg: string, ...args: any[]): void; warn(msg: string, ...args: any[]): void };
  };
  export function createTemplateAction<Input = any, Output = any>(opts: {
    id: string;
    description?: string;
    schema?: any;
    handler: (ctx: ActionContext<Input, Output>) => Promise<void> | void;
  }): any;
}

declare module '@backstage/backend-plugin-api' {
  export function createBackendPlugin(opts: any): any;
  export const coreServices: any;
}

declare module '@backstage/backend-common' {
  export function errorHandler(): any;
}

declare module '@backstage/catalog-client' {
  export class CatalogClient {
    constructor(opts: any);
    getEntities(opts?: any): Promise<{ items: any[] }>;
  }
}

declare module '@backstage/config' {
  export type Config = {
    getString(key: string): string;
    getOptionalString(key: string): string | undefined;
  };
}

declare module '@backstage/errors' {
  export class NotFoundError extends Error {}
  export class InputError extends Error {}
}

declare module '@backstage/dev-utils' {
  export function createDevApp(): {
    registerPlugin(plugin: any): any;
    addPage(opts: { element: any; title: string; path: string }): any;
    render(): void;
  };
}

declare module '@mui/material';
declare module '@mui/material/*';
declare module '@mui/icons-material';
declare module '@mui/icons-material/*';
declare module '@emotion/react';
declare module '@emotion/styled';

declare module 'react-router-dom' {
  export function useParams<T = Record<string, string | undefined>>(): T;
  export function useNavigate(): (path: string) => void;
  export const Route: any;
  export const Routes: any;
}

declare module 'express' {
  export type Request = any;
  export type Response = any;
  export type Router = any;
  export default function express(): any;
}
