import { after, before, beforeEach, test } from "node:test"
import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import { fileURLToPath } from "node:url"

import {
  assertFails,
  assertSucceeds,
  initializeTestEnvironment,
} from "@firebase/rules-unit-testing"
import {
  collection,
  deleteDoc,
  doc,
  documentId,
  getCountFromServer,
  getDoc,
  getDocs,
  query,
  setDoc,
  updateDoc,
  where,
} from "firebase/firestore"

const projectId = "demo-yokai"
const rulesPath = fileURLToPath(new URL("../../firestore.rules", import.meta.url))
const emulatorAddress = process.env.FIRESTORE_EMULATOR_HOST ?? "127.0.0.1:8088"
const separator = emulatorAddress.lastIndexOf(":")
const emulatorHost = emulatorAddress.slice(0, separator)
const emulatorPort = Number(emulatorAddress.slice(separator + 1))

const validEnvelope = {
  version: 1,
  cipher: "aes-256-gcm",
  kdf: "argon2id-3-65536-4",
  salt: "A".repeat(22),
  nonce: "A".repeat(16),
  ciphertext: "A".repeat(56),
}

let testEnvironment

before(async () => {
  testEnvironment = await initializeTestEnvironment({
    projectId,
    firestore: {
      rules: readFileSync(rulesPath, "utf8"),
      host: emulatorHost,
      port: emulatorPort,
    },
  })
})

beforeEach(async () => {
  await testEnvironment.clearFirestore()
})

after(async () => {
  await testEnvironment.cleanup()
})

function configDoc(firestore, userId = "owner-user") {
  return doc(firestore, "device_configs", userId)
}

async function seedConfig(userId = "owner-user", value = validEnvelope) {
  await testEnvironment.withSecurityRulesDisabled(async (context) => {
    await setDoc(configDoc(context.firestore(), userId), value)
  })
}

async function storedConfig(userId = "owner-user") {
  let result
  await testEnvironment.withSecurityRulesDisabled(async (context) => {
    const snapshot = await getDoc(configDoc(context.firestore(), userId))
    result = snapshot.exists() ? snapshot.data() : null
  })
  return result
}

function withoutField(field) {
  const value = { ...validEnvelope }
  delete value[field]
  return value
}

test("owner can create, get, update, and delete an encrypted config", async () => {
  const owner = testEnvironment.authenticatedContext("owner-user").firestore()
  const target = configDoc(owner)

  await assertSucceeds(setDoc(target, validEnvelope))
  const created = await assertSucceeds(getDoc(target))
  assert.deepEqual(created.data(), validEnvelope)

  const nextCiphertext = "B".repeat(56)
  await assertSucceeds(updateDoc(target, { ciphertext: nextCiphertext }))
  const updated = await assertSucceeds(getDoc(target))
  assert.equal(updated.data().ciphertext, nextCiphertext)

  await assertSucceeds(deleteDoc(target))
  assert.equal(await storedConfig(), null)
})

test("another user cannot create, read, overwrite, update, or delete owner data", async () => {
  await seedConfig()
  const intruder = testEnvironment.authenticatedContext("intruder-user").firestore()
  const target = configDoc(intruder)

  await assertFails(getDoc(target))
  await assertFails(setDoc(target, validEnvelope))
  await assertFails(updateDoc(target, { ciphertext: "B".repeat(56) }))
  await assertFails(deleteDoc(target))
  await assertFails(setDoc(configDoc(intruder, "absent-user"), validEnvelope))

  assert.deepEqual(await storedConfig(), validEnvelope)
  assert.equal(await storedConfig("absent-user"), null)
})

test("unauthenticated clients cannot create, read, update, or delete configs", async () => {
  await seedConfig()
  const anonymous = testEnvironment.unauthenticatedContext().firestore()
  const target = configDoc(anonymous)

  await assertFails(getDoc(target))
  await assertFails(setDoc(configDoc(anonymous, "absent-user"), validEnvelope))
  await assertFails(updateDoc(target, { ciphertext: "B".repeat(56) }))
  await assertFails(deleteDoc(target))

  assert.deepEqual(await storedConfig(), validEnvelope)
  assert.equal(await storedConfig("absent-user"), null)
})

test("collection lists, constrained queries, and aggregations are denied", async () => {
  await seedConfig()
  for (const userId of ["owner-user", "intruder-user"]) {
    const firestore = testEnvironment.authenticatedContext(userId).firestore()
    const configs = collection(firestore, "device_configs")
    await assertFails(getDocs(configs))
    await assertFails(getDocs(query(configs, where(documentId(), "==", "owner-user"))))
    await assertFails(getCountFromServer(configs))
  }
})

test("device config subcollections remain inaccessible", async () => {
  await seedConfig()
  const owner = testEnvironment.authenticatedContext("owner-user").firestore()
  const child = doc(owner, "device_configs", "owner-user", "private", "child")

  await assertFails(setDoc(child, { value: "not allowed" }))
  await assertFails(getDoc(child))
})

test("owner can delete a malformed legacy document", async () => {
  await seedConfig("owner-user", { plaintext: "legacy" })
  const owner = testEnvironment.authenticatedContext("owner-user").firestore()

  await assertSucceeds(deleteDoc(configDoc(owner)))
  assert.equal(await storedConfig(), null)
})

const invalidEnvelopes = [
  ...["version", "cipher", "kdf", "salt", "nonce", "ciphertext"].map((field) => [
    `missing ${field}`,
    withoutField(field),
  ]),
  ["unexpected field", { ...validEnvelope, email: "leak@example.com" }],
  ["string version", { ...validEnvelope, version: "1" }],
  ["boolean version", { ...validEnvelope, version: true }],
  ["fractional version", { ...validEnvelope, version: 1.5 }],
  ["unsupported version", { ...validEnvelope, version: 2 }],
  ["non-string cipher", { ...validEnvelope, cipher: 1 }],
  ["empty cipher", { ...validEnvelope, cipher: "" }],
  ["unsupported cipher", { ...validEnvelope, cipher: "plaintext" }],
  ["non-string KDF", { ...validEnvelope, kdf: false }],
  ["empty KDF", { ...validEnvelope, kdf: "" }],
  ["unsupported KDF", { ...validEnvelope, kdf: "argon2id" }],
  ["non-string salt", { ...validEnvelope, salt: 1 }],
  ["empty salt", { ...validEnvelope, salt: "" }],
  ["short salt", { ...validEnvelope, salt: "A".repeat(21) }],
  ["long salt", { ...validEnvelope, salt: "A".repeat(23) }],
  ["padded salt", { ...validEnvelope, salt: `${"A".repeat(21)}=` }],
  ["URL-safe salt", { ...validEnvelope, salt: `${"A".repeat(21)}-` }],
  ["whitespace salt", { ...validEnvelope, salt: `${"A".repeat(21)} ` }],
  ["Unicode salt", { ...validEnvelope, salt: `${"A".repeat(21)}é` }],
  ["non-string nonce", { ...validEnvelope, nonce: [] }],
  ["empty nonce", { ...validEnvelope, nonce: "" }],
  ["short nonce", { ...validEnvelope, nonce: "A".repeat(15) }],
  ["long nonce", { ...validEnvelope, nonce: "A".repeat(17) }],
  ["padded nonce", { ...validEnvelope, nonce: `${"A".repeat(15)}=` }],
  ["URL-safe nonce", { ...validEnvelope, nonce: `${"A".repeat(15)}_` }],
  ["whitespace nonce", { ...validEnvelope, nonce: `${"A".repeat(15)} ` }],
  ["Unicode nonce", { ...validEnvelope, nonce: `${"A".repeat(15)}é` }],
  ["non-string ciphertext", { ...validEnvelope, ciphertext: { value: "A" } }],
  ["empty ciphertext", { ...validEnvelope, ciphertext: "" }],
  ["cryptographically short ciphertext", { ...validEnvelope, ciphertext: "A".repeat(55) }],
  ["invalid raw-base64 length", { ...validEnvelope, ciphertext: "A".repeat(57) }],
  ["oversized ciphertext", { ...validEnvelope, ciphertext: "A".repeat(900001) }],
  ["padded ciphertext", { ...validEnvelope, ciphertext: `${"A".repeat(55)}=` }],
  ["URL-safe ciphertext", { ...validEnvelope, ciphertext: `${"A".repeat(55)}-` }],
  ["whitespace ciphertext", { ...validEnvelope, ciphertext: `${"A".repeat(55)} ` }],
  ["Unicode ciphertext", { ...validEnvelope, ciphertext: `${"A".repeat(55)}é` }],
]

for (const [name, invalidEnvelope] of invalidEnvelopes) {
  test(`invalid ${name} create and update fail without changing state`, async () => {
    const owner = testEnvironment.authenticatedContext("owner-user").firestore()
    const target = configDoc(owner)

    await assertFails(setDoc(target, invalidEnvelope))
    assert.equal(await storedConfig(), null)

    await seedConfig()
    await assertFails(setDoc(target, invalidEnvelope))
    assert.deepEqual(await storedConfig(), validEnvelope)
  })
}

for (const ciphertextLength of [56, 900000]) {
  test(`ciphertext boundary ${ciphertextLength} is accepted`, async () => {
    const owner = testEnvironment.authenticatedContext("owner-user").firestore()
    const value = { ...validEnvelope, ciphertext: "A".repeat(ciphertextLength) }

    await assertSucceeds(setDoc(configDoc(owner), value))
    assert.deepEqual(await storedConfig(), value)
  })
}
