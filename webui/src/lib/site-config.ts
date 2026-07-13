const rawSiteName = process.env.NEXT_PUBLIC_SITE_NAME?.trim();
const rawSiteTitle = process.env.NEXT_PUBLIC_SITE_TITLE?.trim();
const rawSiteDescription = process.env.NEXT_PUBLIC_SITE_DESCRIPTION?.trim();
const rawSiteIcon = process.env.NEXT_PUBLIC_SITE_ICON?.trim();

export const SITE_NAME = rawSiteName || "Twilight";
export const SITE_TITLE = rawSiteTitle || `${SITE_NAME} - Emby 管理系统`;
export const SITE_DESCRIPTION = rawSiteDescription || `${SITE_NAME} 的 Emby/Jellyfin 管理系统`;
export const SITE_ICON = rawSiteIcon || "/favicon.png";
