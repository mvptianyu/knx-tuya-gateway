/**
 * Navigates to the specified scene in the Ray Play Cool Functional app.
 *
 * @param {string} deviceId - The ID of the device to navigate to.
 * @param {string} groupId - The ID of the group to navigate to.
 * @return {void}
 */
export const jumpToCoolBar = (deviceId, groupId) => {
  if (!deviceId && !groupId) {
    console.warn('jumpToCoolBar deviceId and groupId cannot be empty');
    return;
  }
  const jumpUrl = `functional://rayPlayCoolFunctional/home?deviceId=${deviceId}&groupId=${groupId}`;
  ty.navigateTo({
    url: jumpUrl,
    fail(e){console.log(e)},
    success(e){console.log(e)}
  });
};

export default jumpToCoolBar;
