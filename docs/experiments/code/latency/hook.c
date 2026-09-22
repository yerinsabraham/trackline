#include <stdio.h>
#include <string.h>
#include <stdlib.h>
int main(void) {
    static char buf[1 << 20];
    size_t n = fread(buf, 1, sizeof(buf) - 1, stdin);
    buf[n] = 0;
    if (strstr(buf, "secrets.env")) {
        fputs("BLOCKED: secrets.env is off limits. Use config.local.json instead.", stderr);
        return 2;
    }
    return 0;
}
