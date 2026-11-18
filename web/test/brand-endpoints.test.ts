import { expect, test } from "bun:test";
import { isLuckyTVEndpoint } from "../src/lib/luckytv-video-contracts";
import { luckyTVConnectionLabel } from "../src/services/api/luckytv-connection";
import { channelRequest } from "../src/services/api/custom-channel-relay";

test("both brands retain strict managed endpoint recognition and relay credentials", () => {
    const baseUrl = "https://luckytv.app";
    expect(isLuckyTVEndpoint(baseUrl + "/v1")).toBe(true);
    const request = channelRequest({baseUrl, apiKey: "", apiFormat: "openai", credentialRef: "luckytv"}, baseUrl + "/v1/models");
    expect(request.url).toContain("/ai/custom");
    expect(JSON.stringify(request)).not.toContain("Bearer ");
    for (const invalidUrl of ["https://legacy-enterprise.example.com", "http://luckytv.app", "https://luckytv.app:444", "https://luckytv.app.evil.test", "https://user@luckytv.app", "https://evil.test/luckytv.app"]) {
        expect(isLuckyTVEndpoint(invalidUrl)).toBe(false);
    }
});

test("connected identity prefers email over display name and username", () => {
    expect(luckyTVConnectionLabel({state: "connected", account: {id: "1", email: "creator@example.com", display_name: "Creator", username: "legacy"}})).toBe("已连接 creator@example.com");
});
