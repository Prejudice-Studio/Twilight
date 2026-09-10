import type { UserInfo } from "$lib/types";

declare global {
  namespace App {
    interface Locals {
      user: UserInfo | null;
    }

    interface PageData {
      user: UserInfo | null;
    }
  }
}

export {};
