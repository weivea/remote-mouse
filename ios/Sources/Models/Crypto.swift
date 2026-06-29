import Foundation
import CryptoKit

enum Crypto {
    // PBKDF2-HMAC-SHA256 to match server (golang.org/x/crypto/pbkdf2).
    static func pbkdf2(password: String, salt: Data, iterations: Int, keyLen: Int) -> Data {
        let key = SymmetricKey(data: Data(password.utf8))
        var derived = Data()
        var block: UInt32 = 1
        while derived.count < keyLen {
            var u = Data(salt)
            withUnsafeBytes(of: block.bigEndian) { u.append(contentsOf: $0) }
            var prev = Data(HMAC<SHA256>.authenticationCode(for: u, using: key))
            var t = prev
            for _ in 1..<iterations {
                prev = Data(HMAC<SHA256>.authenticationCode(for: prev, using: key))
                for i in 0..<t.count { t[i] ^= prev[i] }
            }
            derived.append(t)
            block += 1
        }
        return derived.prefix(keyLen)
    }

    static func proof(password: String, salt: Data, nonce: Data, iterations: Int) -> String {
        let key = pbkdf2(password: password, salt: salt, iterations: iterations, keyLen: 32)
        let mac = HMAC<SHA256>.authenticationCode(for: nonce, using: SymmetricKey(data: key))
        return Data(mac).base64EncodedString()
    }
}
